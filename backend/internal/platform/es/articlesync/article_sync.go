package articlesync

import (
	"fmt"
	"strconv"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"gorm.io/gorm"
	"myblogx/internal/platform/es"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/markdown"
)

type articleESTop struct {
	AdminTop  bool
	AuthorTop bool
}

func toESID(id ctype.ID) string {
	return id.String()
}

func toESNullableID(id *ctype.ID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

// ArticleSearchProjectionEventType 定义文章搜索读模型的变更类型。
type ArticleSearchProjectionEventType string

const (
	ArticleSearchProjectionArticleUpsert      ArticleSearchProjectionEventType = "article_upsert"
	ArticleSearchProjectionArticleDelete      ArticleSearchProjectionEventType = "article_delete"
	ArticleSearchProjectionAuthorSnapshot     ArticleSearchProjectionEventType = "author_snapshot"
	ArticleSearchProjectionCategorySnapshot   ArticleSearchProjectionEventType = "category_snapshot"
	ArticleSearchProjectionTagSnapshot        ArticleSearchProjectionEventType = "tag_snapshot"
	ArticleSearchProjectionArticleTagsChanged ArticleSearchProjectionEventType = "article_tags_changed"
	ArticleSearchProjectionArticleTopChanged  ArticleSearchProjectionEventType = "article_top_changed"
	ArticleSearchProjectionTopUserChanged     ArticleSearchProjectionEventType = "top_user_changed"
)

// ArticleSearchProjectionEvent 描述一次 ES 读模型同步事件。
type ArticleSearchProjectionEvent struct {
	Type ArticleSearchProjectionEventType
	IDs  []ctype.ID
}

// ArticleModelDelta 描述 article_models 单行 update 的“变更列快照”。
// Changed 的 key 使用数据库字段名（snake_case），value 为变更后的值。
type ArticleModelDelta struct {
	ArticleID ctype.ID
	Changed   map[string]any
}

// ArticleRowSnapshot 是文章 ES 投影所需的“文章主体快照”。
// insert 场景直接来自 binlog after row；补偿重建场景则从 article_models 按需读取。
type ArticleRowSnapshot struct {
	ID               ctype.ID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Title            string
	Abstract         string
	Content          string
	CategoryID       *ctype.ID
	Cover            string
	AuthorID         ctype.ID
	ViewCount        int
	DiggCount        int
	CommentCount     int
	FavorCount       int
	CommentsToggle   bool
	PublishStatus    enum.ArticleStatus
	VisibilityStatus enum.ArticleVisibilityStatus
}

type articleProjectionDeps struct {
	CategoryTitleMap map[ctype.ID]string
	AuthorMap        map[ctype.ID]authorSnapshot
	ArticleTagsMap   map[ctype.ID][]models.ESTag
	TopMap           map[ctype.ID]articleESTop
}

// SyncArticleSearchProjection 是文章搜索读模型的统一更新入口。
// River 在监听到相关表变更后，统一通过该入口路由到“单字段或小范围字段”更新逻辑。
func SyncArticleSearchProjection(db *gorm.DB, client *elasticsearch.Client, event ArticleSearchProjectionEvent) error {
	switch event.Type {
	case ArticleSearchProjectionArticleUpsert:
		return SyncESDocs(db, client, event.IDs)
	case ArticleSearchProjectionArticleDelete:
		return DeleteESDocs(db, client, event.IDs)
	case ArticleSearchProjectionAuthorSnapshot:
		return SyncESDocsByAuthorIDs(db, client, event.IDs)
	case ArticleSearchProjectionCategorySnapshot:
		return SyncESDocsByCategoryIDs(db, client, event.IDs)
	case ArticleSearchProjectionTagSnapshot:
		return UpdateESDocsTagsByTagIDs(db, client, event.IDs)
	case ArticleSearchProjectionArticleTagsChanged:
		return UpdateESDocsTags(db, client, event.IDs)
	case ArticleSearchProjectionArticleTopChanged:
		return UpdateESDocsTop(db, client, event.IDs)
	case ArticleSearchProjectionTopUserChanged:
		return UpdateESDocsTopByTopUserIDs(db, client, event.IDs)
	default:
		return nil
	}
}

// UpdateESDocsByArticleDeltas 按 article_models 的“变更列”做局部更新。
// 说明：
// 1. 仅更新真正变更的展示字段，避免每次 update 都整篇重建 ES 文档。
// 2. 对 category/author 这类快照字段，会批量查询必要信息后再补齐。
// 3. 当 author_id 变更时，会同步重算 top 字段，确保 author_top 语义正确。
func UpdateESDocsByArticleDeltas(db *gorm.DB, client *elasticsearch.Client, deltas []ArticleModelDelta) error {
	if db == nil || client == nil {
		return nil
	}

	deltas = normalizeArticleModelDeltas(deltas)
	if len(deltas) == 0 {
		return nil
	}

	categoryIDs := make([]ctype.ID, 0, len(deltas))
	authorIDs := make([]ctype.ID, 0, len(deltas))
	topRecalcArticleIDs := make([]ctype.ID, 0, len(deltas))
	for _, delta := range deltas {
		if rawCategoryID, ok := delta.Changed["category_id"]; ok {
			if categoryID, ok := scanNullableIDValue(rawCategoryID); ok && categoryID != nil {
				categoryIDs = append(categoryIDs, *categoryID)
			}
		}
		if rawAuthorID, ok := delta.Changed["author_id"]; ok {
			if authorID, ok := scanIDValue(rawAuthorID); ok && authorID != 0 {
				authorIDs = append(authorIDs, authorID)
				topRecalcArticleIDs = append(topRecalcArticleIDs, delta.ArticleID)
			}
		}
	}

	categoryTitleMap, err := loadCategoryTitleMap(db, categoryIDs)
	if err != nil {
		return err
	}
	authorMap, err := loadAuthorSnapshotMap(db, authorIDs)
	if err != nil {
		return err
	}
	topMap, err := loadArticleESTopMap(db, topRecalcArticleIDs)
	if err != nil {
		return err
	}

	reqs := make([]*es_service.BulkRequest, 0, len(deltas))
	for _, delta := range deltas {
		data := make(map[string]any)
		for field, raw := range delta.Changed {
			switch field {
			case "created_at":
				if value, ok := scanTimeValue(raw); ok {
					data["created_at"] = value
				}
			case "updated_at":
				if value, ok := scanTimeValue(raw); ok {
					data["updated_at"] = value
				}
			case "title":
				if value, ok := scanStringValue(raw); ok {
					data["title"] = value
				}
			case "abstract":
				if value, ok := scanStringValue(raw); ok {
					data["abstract"] = value
				}
			case "content":
				if content, ok := scanStringValue(raw); ok {
					data["content_parts"] = markdown.MdToContentParts(content)
					data["content_head"] = markdown.ExtractText(markdown.MdToTextParagraph(content), 150)
				}
			case "category_id":
				if categoryID, ok := scanNullableIDValue(raw); ok {
					categoryIDValue := toESNullableID(categoryID)
					var categoryTitle string
					if categoryID != nil {
						categoryTitle = categoryTitleMap[*categoryID]
					}
					data["category_id"] = categoryIDValue
					data["category"] = map[string]any{
						"id":    categoryIDValue,
						"title": categoryTitle,
					}
				}
			case "cover":
				if value, ok := scanStringValue(raw); ok {
					data["cover"] = value
				}
			case "author_id":
				if authorID, ok := scanIDValue(raw); ok {
					author := authorMap[authorID]
					data["author_id"] = toESID(authorID)
					data["author"] = map[string]any{
						"id":       toESID(authorID),
						"nickname": author.Nickname,
						"avatar":   author.Avatar,
					}
				}
			case "view_count":
				if value, ok := scanIntValue(raw); ok {
					data["view_count"] = value
				}
			case "digg_count":
				if value, ok := scanIntValue(raw); ok {
					data["digg_count"] = value
				}
			case "comment_count":
				if value, ok := scanIntValue(raw); ok {
					data["comment_count"] = value
				}
			case "favor_count":
				if value, ok := scanIntValue(raw); ok {
					data["favor_count"] = value
				}
			case "comments_toggle":
				if value, ok := scanBoolValue(raw); ok {
					data["comments_toggle"] = value
				}
			case "publish_status":
				if value, ok := scanIntValue(raw); ok {
					data["publish_status"] = enum.ArticleStatus(value)
				}
			case "visibility_status":
				if value, ok := scanStringValue(raw); ok {
					data["visibility_status"] = enum.ArticleVisibilityStatus(value)
				}
			}
		}

		if _, authorChanged := delta.Changed["author_id"]; authorChanged {
			top := topMap[delta.ArticleID]
			data["top"] = map[string]any{
				"user":  top.AuthorTop,
				"admin": top.AdminTop,
			}
			data["admin_top"] = top.AdminTop
			data["author_top"] = top.AuthorTop
		}

		if len(data) == 0 {
			continue
		}
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
			ID:     strconv.FormatUint(uint64(delta.ArticleID), 10),
			Data:   data,
		})
	}

	return applyArticlePartialBulkUpdate(db, client, reqs, "按 article_models 差异更新 ES 文档失败")
}

// BuildArticleESDocument 将文章及其聚合字段转换为 ES 文档。
func BuildArticleESDocument(article models.ArticleModel, adminTop, authorTop bool) map[string]any {
	tags := make([]models.ESTag, 0, len(article.Tags))
	for _, tag := range article.Tags {
		tags = append(tags, models.ESTag{
			ID:    tag.ID,
			Title: tag.Title,
		})
	}

	snapshot := ArticleRowSnapshot{
		ID:               article.ID,
		CreatedAt:        article.CreatedAt,
		UpdatedAt:        article.UpdatedAt,
		Title:            article.Title,
		Abstract:         article.Abstract,
		Content:          article.Content,
		CategoryID:       article.CategoryID,
		Cover:            article.Cover,
		AuthorID:         article.AuthorID,
		ViewCount:        article.ViewCount,
		DiggCount:        article.DiggCount,
		CommentCount:     article.CommentCount,
		FavorCount:       article.FavorCount,
		CommentsToggle:   article.CommentsToggle,
		PublishStatus:    article.PublishStatus,
		VisibilityStatus: article.VisibilityStatus,
	}

	deps := articleProjectionDeps{
		CategoryTitleMap: map[ctype.ID]string{},
		AuthorMap:        map[ctype.ID]authorSnapshot{},
		ArticleTagsMap:   map[ctype.ID][]models.ESTag{article.ID: tags},
		TopMap: map[ctype.ID]articleESTop{
			article.ID: {
				AdminTop:  adminTop,
				AuthorTop: authorTop,
			},
		},
	}
	if article.CategoryModel != nil && article.CategoryID != nil {
		deps.CategoryTitleMap[*article.CategoryID] = article.CategoryModel.Title
	}
	if article.UserModel.ID != 0 {
		deps.AuthorMap[article.AuthorID] = authorSnapshot{
			Nickname: article.UserModel.Nickname,
			Avatar:   article.UserModel.Avatar,
		}
	}
	return buildArticleESDocumentFromSnapshot(snapshot, deps)
}

func buildArticleESDocumentFromSnapshot(snapshot ArticleRowSnapshot, deps articleProjectionDeps) map[string]any {
	categoryIDValue := toESNullableID(snapshot.CategoryID)
	var categoryTitle string
	if snapshot.CategoryID != nil {
		categoryTitle = deps.CategoryTitleMap[*snapshot.CategoryID]
	}
	author := deps.AuthorMap[snapshot.AuthorID]
	top := deps.TopMap[snapshot.ID]
	tags := deps.ArticleTagsMap[snapshot.ID]
	if tags == nil {
		tags = []models.ESTag{}
	}

	return map[string]any{
		"id":            toESID(snapshot.ID),
		"created_at":    snapshot.CreatedAt,
		"updated_at":    snapshot.UpdatedAt,
		"title":         snapshot.Title,
		"abstract":      snapshot.Abstract,
		"content_parts": markdown.MdToContentParts(snapshot.Content),
		"content_head":  markdown.ExtractText(markdown.MdToTextParagraph(snapshot.Content), 150),
		"category_id":   categoryIDValue,
		"category": map[string]any{
			"id":    categoryIDValue,
			"title": categoryTitle,
		},
		"cover":     snapshot.Cover,
		"author_id": toESID(snapshot.AuthorID),
		"author": map[string]any{
			"id":       toESID(snapshot.AuthorID),
			"nickname": author.Nickname,
			"avatar":   author.Avatar,
		},
		"view_count":        snapshot.ViewCount,
		"digg_count":        snapshot.DiggCount,
		"comment_count":     snapshot.CommentCount,
		"favor_count":       snapshot.FavorCount,
		"publish_status":    snapshot.PublishStatus,
		"visibility_status": snapshot.VisibilityStatus,
		"comments_toggle":   snapshot.CommentsToggle,
		"tags":              tags,
		"top": map[string]any{
			"user":  top.AuthorTop,
			"admin": top.AdminTop,
		},
		"admin_top":  top.AdminTop,
		"author_top": top.AuthorTop,
	}
}

// SyncESDocsByArticleSnapshots 直接使用文章主体快照构建 ES 文档。
// 文章 insert 主路径会使用该入口，避免再回查 article_models。
func SyncESDocsByArticleSnapshots(db *gorm.DB, client *elasticsearch.Client, snapshots []ArticleRowSnapshot) error {
	if db == nil || client == nil {
		return nil
	}
	snapshots = normalizeArticleSnapshots(snapshots)
	if len(snapshots) == 0 {
		return nil
	}
	return indexArticleSnapshots(db, client, snapshots, "同步文章 ES 文档失败")
}

// SyncESDocs 按文章 ID 批量重建 ES 文档。
// 这里会从数据库重新读取文章、标签和置顶信息，再统一索引到 ES。
func SyncESDocs(db *gorm.DB, client *elasticsearch.Client, articleIDs []ctype.ID) error {
	if db == nil || client == nil {
		return nil
	}

	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 {
		return nil
	}

	snapshots, err := loadArticleSnapshotsByIDs(db, articleIDs)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		return fmt.Errorf("按文章 ID 同步 ES 文档失败: 未加载到任何文章，article_ids=%v", articleIDs)
	}

	missingArticleIDs := collectMissingArticleIDs(articleIDs, snapshots)
	if len(missingArticleIDs) > 0 {
		return fmt.Errorf("按文章 ID 同步 ES 文档失败: 部分文章未加载到，missing_article_ids=%v", missingArticleIDs)
	}
	return indexArticleSnapshots(db, client, snapshots, "同步文章 ES 文档失败")
}

func indexArticleSnapshots(db *gorm.DB, client *elasticsearch.Client, snapshots []ArticleRowSnapshot, errPrefix string) error {
	if len(snapshots) == 0 {
		return nil
	}

	deps, err := loadArticleProjectionDeps(db, snapshots)
	if err != nil {
		return err
	}

	reqs := make([]*es_service.BulkRequest, 0, len(snapshots))
	for _, snapshot := range snapshots {
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionIndex,
			ID:     strconv.FormatUint(uint64(snapshot.ID), 10),
			Data:   buildArticleESDocumentFromSnapshot(snapshot, deps),
		})
	}

	resp := es_service.IndexBulk(client, models.ArticleModel{}.Index(), reqs)
	if !resp.Success {
		return fmt.Errorf("%s: %s", errPrefix, resp.Msg)
	}
	if data, ok := resp.Data.(map[string]any); ok {
		if hasErrors, ok := data["errors"].(bool); ok && hasErrors {
			return fmt.Errorf("%s: bulk errors", errPrefix)
		}
	}
	return nil
}
