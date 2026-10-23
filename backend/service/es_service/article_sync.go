package es_service

import (
	"encoding/json"
	"fmt"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/markdown"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"gorm.io/gorm"
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

	reqs := make([]*BulkRequest, 0, len(deltas))
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
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
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

	reqs := make([]*BulkRequest, 0, len(snapshots))
	for _, snapshot := range snapshots {
		reqs = append(reqs, &BulkRequest{
			Action: ActionIndex,
			ID:     strconv.FormatUint(uint64(snapshot.ID), 10),
			Data:   buildArticleESDocumentFromSnapshot(snapshot, deps),
		})
	}

	resp := IndexBulk(client, models.ArticleModel{}.Index(), reqs)
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

func collectMissingArticleIDs(requested []ctype.ID, snapshots []ArticleRowSnapshot) []ctype.ID {
	if len(requested) == 0 {
		return nil
	}
	existing := make(map[ctype.ID]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		existing[snapshot.ID] = struct{}{}
	}
	missing := make([]ctype.ID, 0)
	for _, articleID := range requested {
		if _, ok := existing[articleID]; ok {
			continue
		}
		missing = append(missing, articleID)
	}
	return missing
}

func normalizeArticleSnapshots(snapshots []ArticleRowSnapshot) []ArticleRowSnapshot {
	if len(snapshots) == 0 {
		return nil
	}

	merged := make(map[ctype.ID]ArticleRowSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.ID == 0 {
			continue
		}
		merged[snapshot.ID] = snapshot
	}
	if len(merged) == 0 {
		return nil
	}

	articleIDs := make([]ctype.ID, 0, len(merged))
	for articleID := range merged {
		articleIDs = append(articleIDs, articleID)
	}
	slices.Sort(articleIDs)

	result := make([]ArticleRowSnapshot, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		result = append(result, merged[articleID])
	}
	return result
}

// UpdateESDocsTags 在文章标签关系变化后刷新对应文章的 ES 文档。
func UpdateESDocsTags(db *gorm.DB, client *elasticsearch.Client, articleIDs []ctype.ID) error {
	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 || db == nil || client == nil {
		return nil
	}

	articleList, err := loadArticlesForESTags(db, articleIDs)
	if err != nil {
		return err
	}

	reqs := make([]*BulkRequest, 0, len(articleList))
	for _, article := range articleList {
		tags := make([]models.ESTag, 0, len(article.Tags))
		for _, tag := range article.Tags {
			tags = append(tags, models.ESTag{
				ID:    tag.ID,
				Title: tag.Title,
			})
		}
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
			ID:     strconv.FormatUint(uint64(article.ID), 10),
			Data: map[string]any{
				"tags":       tags,
				"updated_at": article.UpdatedAt,
			},
		})
	}

	return applyArticlePartialBulkUpdate(db, client, reqs, "更新文章 ES 标签失败")
}

// UpdateESDocsContent 在文章正文变化后刷新对应文章的 ES 文档。
func UpdateESDocsContent(db *gorm.DB, client *elasticsearch.Client, articleIDs []ctype.ID) error {
	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 || db == nil || client == nil {
		return nil
	}

	articleList, err := loadArticlesForESContentUpdate(db, articleIDs)
	if err != nil {
		return err
	}

	reqs := make([]*BulkRequest, 0, len(articleList))
	for _, article := range articleList {
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
			ID:     strconv.FormatUint(uint64(article.ID), 10),
			Data: map[string]any{
				"updated_at":    article.UpdatedAt,
				"title":         article.Title,
				"abstract":      article.Abstract,
				"content_parts": markdown.MdToContentParts(article.Content),
				"content_head":  markdown.ExtractText(markdown.MdToTextParagraph(article.Content), 150),
				"category_id":   toESNullableID(article.CategoryID),
				"category": map[string]any{
					"id": toESNullableID(article.CategoryID),
					"title": func() string {
						if article.CategoryModel == nil {
							return ""
						}
						return article.CategoryModel.Title
					}(),
				},
				"cover":     article.Cover,
				"author_id": toESID(article.AuthorID),
				"author": map[string]any{
					"id": toESID(article.AuthorID),
					"nickname": func() string {
						if article.UserModel.ID == 0 {
							return ""
						}
						return article.UserModel.Nickname
					}(),
					"avatar": func() string {
						if article.UserModel.ID == 0 {
							return ""
						}
						return article.UserModel.Avatar
					}(),
				},
				"publish_status":    article.PublishStatus,
				"visibility_status": article.VisibilityStatus,
				"comments_toggle":   article.CommentsToggle,
			},
		})
	}

	return applyArticlePartialBulkUpdate(db, client, reqs, "更新文章 ES 正文失败")
}

// UpdateESDocsTop 在文章置顶状态变化后刷新对应文章的 ES 文档。
func UpdateESDocsTop(db *gorm.DB, client *elasticsearch.Client, articleIDs []ctype.ID) error {
	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 || db == nil || client == nil {
		return nil
	}

	topMap, err := loadArticleESTopMap(db, articleIDs)
	if err != nil {
		return err
	}

	reqs := make([]*BulkRequest, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		top := topMap[articleID]
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
			ID:     strconv.FormatUint(uint64(articleID), 10),
			Data: map[string]any{
				"top": map[string]any{
					"user":  top.AuthorTop,
					"admin": top.AdminTop,
				},
				"admin_top":  top.AdminTop,
				"author_top": top.AuthorTop,
			},
		})
	}

	return applyArticlePartialBulkUpdate(db, client, reqs, "更新文章 ES 置顶字段失败")
}

// DeleteESDocs 按文章 ID 删除 ES 文档。
func DeleteESDocs(_ *gorm.DB, client *elasticsearch.Client, articleIDs []ctype.ID) error {
	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 || client == nil {
		return nil
	}

	reqs := make([]*BulkRequest, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		reqs = append(reqs, &BulkRequest{
			Action: ActionDelete,
			ID:     strconv.FormatUint(uint64(articleID), 10),
		})
	}
	return applyArticlePartialBulkUpdate(nil, client, reqs, "删除文章 ES 文档失败")
}

func SyncESDocsByCategoryIDs(db *gorm.DB, client *elasticsearch.Client, categoryIDs []ctype.ID) error {
	if db == nil || client == nil {
		return nil
	}
	categoryIDs = normalizeArticleIDs(categoryIDs)
	if len(categoryIDs) == 0 {
		return nil
	}

	type articleCategoryRow struct {
		ID         ctype.ID
		CategoryID *ctype.ID
	}
	var rows []articleCategoryRow
	if err := db.Model(&models.ArticleModel{}).
		Select("id", "category_id").
		Where("category_id IN ?", categoryIDs).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	affectedCategoryIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.CategoryID != nil {
			affectedCategoryIDs = append(affectedCategoryIDs, *row.CategoryID)
		}
	}
	categoryTitleMap, err := loadCategoryTitleMap(db, affectedCategoryIDs)
	if err != nil {
		return err
	}

	reqs := make([]*BulkRequest, 0, len(rows))
	for _, row := range rows {
		categoryIDValue := toESNullableID(row.CategoryID)
		var categoryTitle string
		if row.CategoryID != nil {
			categoryTitle = categoryTitleMap[*row.CategoryID]
		}
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
			ID:     strconv.FormatUint(uint64(row.ID), 10),
			Data: map[string]any{
				"category_id": categoryIDValue,
				"category": map[string]any{
					"id":    categoryIDValue,
					"title": categoryTitle,
				},
			},
		})
	}
	return applyArticlePartialBulkUpdate(db, client, reqs, "按分类同步文章 ES 分类快照失败")
}

func SyncESDocsByAuthorIDs(db *gorm.DB, client *elasticsearch.Client, userIDs []ctype.ID) error {
	if db == nil || client == nil {
		return nil
	}
	userIDs = normalizeArticleIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}

	type articleAuthorRow struct {
		ID       ctype.ID
		AuthorID ctype.ID
	}
	var rows []articleAuthorRow
	if err := db.Model(&models.ArticleModel{}).
		Select("id", "author_id").
		Where("author_id IN ?", userIDs).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	authorIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		authorIDs = append(authorIDs, row.AuthorID)
	}
	authorMap, err := loadAuthorSnapshotMap(db, authorIDs)
	if err != nil {
		return err
	}

	reqs := make([]*BulkRequest, 0, len(rows))
	for _, row := range rows {
		author := authorMap[row.AuthorID]
		reqs = append(reqs, &BulkRequest{
			Action: ActionUpdate,
			ID:     strconv.FormatUint(uint64(row.ID), 10),
			Data: map[string]any{
				"author_id": toESID(row.AuthorID),
				"author": map[string]any{
					"id":       toESID(row.AuthorID),
					"nickname": author.Nickname,
					"avatar":   author.Avatar,
				},
			},
		})
	}
	return applyArticlePartialBulkUpdate(db, client, reqs, "按作者同步文章 ES 作者快照失败")
}

// UpdateESDocsTagsByTagIDs 在标签信息变化后按 tag_id 批量刷新相关文章的 tags 字段。
func UpdateESDocsTagsByTagIDs(db *gorm.DB, client *elasticsearch.Client, tagIDs []ctype.ID) error {
	if db == nil || client == nil {
		return nil
	}
	tagIDs = normalizeArticleIDs(tagIDs)
	if len(tagIDs) == 0 {
		return nil
	}

	var articleIDs []ctype.ID
	if err := db.Model(&models.ArticleTagModel{}).
		Select("article_id").
		Where("tag_id IN ?", tagIDs).
		Pluck("article_id", &articleIDs).Error; err != nil {
		return err
	}
	return UpdateESDocsTags(db, client, articleIDs)
}

// UpdateESDocsTopByTopUserIDs 在置顶用户信息变化后按 user_id 批量刷新相关文章 top 字段。
func UpdateESDocsTopByTopUserIDs(db *gorm.DB, client *elasticsearch.Client, userIDs []ctype.ID) error {
	if db == nil || client == nil {
		return nil
	}
	userIDs = normalizeArticleIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}

	var articleIDs []ctype.ID
	if err := db.Model(&models.UserTopArticleModel{}).
		Select("article_id").
		Where("user_id IN ?", userIDs).
		Pluck("article_id", &articleIDs).Error; err != nil {
		return err
	}
	return UpdateESDocsTop(db, client, articleIDs)
}

type articleBulkFailure struct {
	Action    string
	ID        string
	Status    int
	ErrorType string
	Reason    string
}

func applyArticlePartialBulkUpdate(db *gorm.DB, client *elasticsearch.Client, reqs []*BulkRequest, errPrefix string) error {
	if len(reqs) == 0 {
		return nil
	}

	resp := IndexBulk(client, models.ArticleModel{}.Index(), reqs)
	if !resp.Success {
		return fmt.Errorf("%s: %s", errPrefix, resp.Msg)
	}

	failures, missingUpdateIDs := collectArticleBulkFailures(resp.Data, reqs, 3)
	if len(missingUpdateIDs) > 0 {
		if err := SyncESDocs(db, client, missingUpdateIDs); err != nil {
			return fmt.Errorf("%s: 文档缺失后补建失败: %w", errPrefix, err)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%s: %s", errPrefix, formatArticleBulkFailureSummary(failures))
	}
	return nil
}

func collectArticleBulkFailures(data any, reqs []*BulkRequest, maxItems int) ([]articleBulkFailure, []ctype.ID) {
	root, ok := data.(map[string]any)
	if !ok {
		return nil, nil
	}
	hasErrors, ok := root["errors"].(bool)
	if !ok || !hasErrors {
		return nil, nil
	}

	items, ok := root["items"].([]any)
	if !ok || len(items) == 0 {
		return []articleBulkFailure{{Reason: "bulk errors"}}, nil
	}

	failures := make([]articleBulkFailure, 0, maxItems)
	missingUpdateIDs := make([]ctype.ID, 0)
	seenMissing := make(map[ctype.ID]struct{})

	for index, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for rawAction, rawResult := range itemMap {
			result, ok := rawResult.(map[string]any)
			if !ok {
				continue
			}
			status := parseBulkStatusValue(result["status"])
			if status >= 200 && status < 300 {
				continue
			}

			action := rawAction
			if index < len(reqs) && reqs[index] != nil && reqs[index].Action != "" {
				action = reqs[index].Action
			}
			docID, _ := result["_id"].(string)
			reason := "unknown"
			errType := ""
			if errObj, ok := result["error"].(map[string]any); ok {
				errType, _ = errObj["type"].(string)
				errReason, _ := errObj["reason"].(string)
				reason = strings.TrimSpace(strings.Join([]string{errType, errReason}, ": "))
				if reason == ":" || reason == "" {
					reason = "unknown"
				}
				if causedBy, ok := errObj["caused_by"].(map[string]any); ok {
					if cbReason, ok := causedBy["reason"].(string); ok && cbReason != "" {
						reason += " (caused_by: " + cbReason + ")"
					}
				}
			}

			if action == ActionDelete && errType == "document_missing_exception" {
				continue
			}
			if action == ActionUpdate && errType == "document_missing_exception" {
				var articleID ctype.ID
				if err := articleID.UnmarshalText([]byte(docID)); err == nil && articleID != 0 {
					if _, ok := seenMissing[articleID]; !ok {
						seenMissing[articleID] = struct{}{}
						missingUpdateIDs = append(missingUpdateIDs, articleID)
					}
					continue
				}
			}

			failures = append(failures, articleBulkFailure{
				Action:    action,
				ID:        docID,
				Status:    status,
				ErrorType: errType,
				Reason:    reason,
			})
			if len(failures) >= maxItems {
				return failures, missingUpdateIDs
			}
		}
	}

	return failures, missingUpdateIDs
}

func parseBulkStatusValue(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return 0
}

func formatArticleBulkFailureSummary(failures []articleBulkFailure) string {
	if len(failures) == 0 {
		return ""
	}

	summaries := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.Action == "" && failure.Reason != "" {
			summaries = append(summaries, failure.Reason)
			continue
		}
		summaries = append(summaries, fmt.Sprintf("%s id=%s status=%d reason=%s", failure.Action, failure.ID, failure.Status, failure.Reason))
	}
	if len(summaries) == 0 {
		return "bulk errors"
	}
	return "bulk errors: " + strings.Join(summaries, " | ")
}

func normalizeArticleModelDeltas(deltas []ArticleModelDelta) []ArticleModelDelta {
	if len(deltas) == 0 {
		return nil
	}

	mergedChangedMap := make(map[ctype.ID]map[string]any, len(deltas))
	for _, delta := range deltas {
		if delta.ArticleID == 0 || len(delta.Changed) == 0 {
			continue
		}
		if _, ok := mergedChangedMap[delta.ArticleID]; !ok {
			mergedChangedMap[delta.ArticleID] = make(map[string]any, len(delta.Changed))
		}
		for field, value := range delta.Changed {
			field = strings.ToLower(strings.TrimSpace(field))
			if field == "" || field == "id" {
				continue
			}
			mergedChangedMap[delta.ArticleID][field] = value
		}
	}

	articleIDs := make([]ctype.ID, 0, len(mergedChangedMap))
	for articleID := range mergedChangedMap {
		articleIDs = append(articleIDs, articleID)
	}
	slices.Sort(articleIDs)

	result := make([]ArticleModelDelta, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		changed := mergedChangedMap[articleID]
		if len(changed) == 0 {
			continue
		}
		result = append(result, ArticleModelDelta{
			ArticleID: articleID,
			Changed:   changed,
		})
	}
	return result
}

func normalizeArticleIDs(articleIDs []ctype.ID) []ctype.ID {
	if len(articleIDs) == 0 {
		return nil
	}

	result := make([]ctype.ID, 0, len(articleIDs))
	seen := make(map[ctype.ID]struct{}, len(articleIDs))
	for _, articleID := range articleIDs {
		if articleID == 0 {
			continue
		}
		if _, ok := seen[articleID]; ok {
			continue
		}
		seen[articleID] = struct{}{}
		result = append(result, articleID)
	}
	slices.Sort(result)
	return result
}

func scanStringValue(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	case []byte:
		return string(value), true
	default:
		return "", false
	}
}

func scanIDValue(raw any) (ctype.ID, bool) {
	var id ctype.ID
	if err := id.Scan(raw); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func scanNullableIDValue(raw any) (*ctype.ID, bool) {
	if raw == nil {
		return nil, true
	}
	id, ok := scanIDValue(raw)
	if !ok {
		return nil, false
	}
	return &id, true
}

func scanIntValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case []byte:
		number, err := strconv.Atoi(string(value))
		if err != nil {
			return 0, false
		}
		return number, true
	case string:
		number, err := strconv.Atoi(value)
		if err != nil {
			return 0, false
		}
		return number, true
	default:
		return 0, false
	}
}

func scanBoolValue(raw any) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case int:
		return value != 0, true
	case int8:
		return value != 0, true
	case int16:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case uint:
		return value != 0, true
	case uint8:
		return value != 0, true
	case uint16:
		return value != 0, true
	case uint32:
		return value != 0, true
	case uint64:
		return value != 0, true
	case []byte:
		text := strings.TrimSpace(strings.ToLower(string(value)))
		return parseBoolText(text)
	case string:
		text := strings.TrimSpace(strings.ToLower(value))
		return parseBoolText(text)
	default:
		return false, false
	}
}

func parseBoolText(value string) (bool, bool) {
	switch value {
	case "1", "true", "t", "yes", "y", "on":
		return true, true
	case "0", "false", "f", "no", "n", "off", "":
		return false, true
	default:
		return false, false
	}
}

func scanTimeValue(raw any) (time.Time, bool) {
	switch value := raw.(type) {
	case time.Time:
		return value, true
	case []byte:
		return parseTimeText(string(value))
	case string:
		return parseTimeText(value)
	default:
		return time.Time{}, false
	}
}

func parseTimeText(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if value, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return value, true
		}
	}
	return time.Time{}, false
}

func loadArticleSnapshotsByIDs(db *gorm.DB, articleIDs []ctype.ID) ([]ArticleRowSnapshot, error) {
	var snapshots []ArticleRowSnapshot
	err := db.Model(&models.ArticleModel{}).Select(
		"id",
		"created_at",
		"updated_at",
		"title",
		"abstract",
		"content",
		"category_id",
		"cover",
		"author_id",
		"view_count",
		"digg_count",
		"comment_count",
		"favor_count",
		"publish_status",
		"visibility_status",
		"comments_toggle",
	).
		Where("id IN ?", articleIDs).
		Order("id asc").
		Find(&snapshots).Error
	return snapshots, err
}

func loadArticleProjectionDeps(db *gorm.DB, snapshots []ArticleRowSnapshot) (articleProjectionDeps, error) {
	deps := articleProjectionDeps{
		CategoryTitleMap: map[ctype.ID]string{},
		AuthorMap:        map[ctype.ID]authorSnapshot{},
		ArticleTagsMap:   map[ctype.ID][]models.ESTag{},
		TopMap:           map[ctype.ID]articleESTop{},
	}
	if len(snapshots) == 0 {
		return deps, nil
	}

	categoryIDs := make([]ctype.ID, 0, len(snapshots))
	authorIDs := make([]ctype.ID, 0, len(snapshots))
	articleIDs := make([]ctype.ID, 0, len(snapshots))
	for _, snapshot := range snapshots {
		articleIDs = append(articleIDs, snapshot.ID)
		authorIDs = append(authorIDs, snapshot.AuthorID)
		if snapshot.CategoryID != nil {
			categoryIDs = append(categoryIDs, *snapshot.CategoryID)
		}
	}

	var err error
	deps.CategoryTitleMap, err = loadCategoryTitleMap(db, categoryIDs)
	if err != nil {
		return deps, err
	}
	deps.AuthorMap, err = loadAuthorSnapshotMap(db, authorIDs)
	if err != nil {
		return deps, err
	}
	deps.ArticleTagsMap, err = loadArticleESTagsMapByArticleIDs(db, articleIDs)
	if err != nil {
		return deps, err
	}
	deps.TopMap, err = loadArticleESTopMapBySnapshots(db, snapshots)
	if err != nil {
		return deps, err
	}
	return deps, nil
}

func loadArticleESTagsMapByArticleIDs(db *gorm.DB, articleIDs []ctype.ID) (map[ctype.ID][]models.ESTag, error) {
	result := make(map[ctype.ID][]models.ESTag)
	articleIDs = normalizeArticleIDs(articleIDs)
	if len(articleIDs) == 0 {
		return result, nil
	}

	type articleTagRow struct {
		ArticleID ctype.ID
		TagID     ctype.ID
		Title     string
		Sort      int
	}
	var rows []articleTagRow
	if err := db.Table("article_tag_models").
		Select("article_tag_models.article_id", "tag_models.id AS tag_id", "tag_models.title", "tag_models.sort").
		Joins("JOIN tag_models ON tag_models.id = article_tag_models.tag_id AND tag_models.deleted_at IS NULL").
		Where("article_tag_models.article_id IN ?", articleIDs).
		Order("article_tag_models.article_id asc, tag_models.sort desc, tag_models.id asc").
		Find(&rows).Error; err != nil {
		return result, err
	}

	for _, row := range rows {
		result[row.ArticleID] = append(result[row.ArticleID], models.ESTag{
			ID:    row.TagID,
			Title: row.Title,
		})
	}
	return result, nil
}

func loadArticleESTopMapBySnapshots(db *gorm.DB, snapshots []ArticleRowSnapshot) (map[ctype.ID]articleESTop, error) {
	result := make(map[ctype.ID]articleESTop, len(snapshots))
	if len(snapshots) == 0 {
		return result, nil
	}

	articleIDs := make([]ctype.ID, 0, len(snapshots))
	articleAuthorMap := make(map[ctype.ID]ctype.ID, len(snapshots))
	for _, snapshot := range snapshots {
		articleIDs = append(articleIDs, snapshot.ID)
		articleAuthorMap[snapshot.ID] = snapshot.AuthorID
	}
	articleIDs = normalizeArticleIDs(articleIDs)

	type topRoleRow struct {
		ArticleID ctype.ID
		TopUserID ctype.ID
		Role      enum.RoleType
	}
	var rows []topRoleRow
	if err := db.Model(&models.UserTopArticleModel{}).
		Select("user_top_article_models.article_id", "user_top_article_models.user_id AS top_user_id", "user_models.role").
		Joins("JOIN user_models ON user_models.id = user_top_article_models.user_id AND user_models.deleted_at IS NULL").
		Where("user_top_article_models.article_id IN ?", articleIDs).
		Order("user_top_article_models.article_id asc, user_top_article_models.user_id asc").
		Find(&rows).Error; err != nil {
		return result, err
	}

	for _, row := range rows {
		state := result[row.ArticleID]
		if row.Role == enum.RoleAdmin {
			state.AdminTop = true
		}
		if row.TopUserID == articleAuthorMap[row.ArticleID] {
			state.AuthorTop = true
		}
		result[row.ArticleID] = state
	}
	return result, nil
}

func loadArticlesForESContentUpdate(db *gorm.DB, articleIDs []ctype.ID) ([]models.ArticleModel, error) {
	var articleList []models.ArticleModel
	err := db.Select(
		"id",
		"updated_at",
		"title",
		"abstract",
		"content",
		"category_id",
		"cover",
		"author_id",
		"publish_status",
		"visibility_status",
		"comments_toggle",
	).
		Where("id IN ?", articleIDs).
		Preload("CategoryModel", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "title")
		}).
		Preload("UserModel", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "nickname", "avatar")
		}).
		Order("id asc").
		Find(&articleList).Error
	return articleList, err
}

func loadArticlesForESTags(db *gorm.DB, articleIDs []ctype.ID) ([]models.ArticleModel, error) {
	var articleList []models.ArticleModel
	if err := db.Select("id", "updated_at").
		Where("id IN ?", articleIDs).
		Order("id asc").
		Find(&articleList).Error; err != nil {
		return nil, err
	}
	if len(articleList) == 0 {
		return articleList, nil
	}

	type relationRow struct {
		ArticleID ctype.ID
		TagID     ctype.ID
	}
	var relationList []relationRow
	if err := db.Model(&models.ArticleTagModel{}).
		Select("article_id", "tag_id").
		Where("article_id IN ?", articleIDs).
		Order("article_id asc").
		Find(&relationList).Error; err != nil {
		return nil, err
	}
	if len(relationList) == 0 {
		return articleList, nil
	}

	tagIDs := make([]ctype.ID, 0, len(relationList))
	for _, relation := range relationList {
		tagIDs = append(tagIDs, relation.TagID)
	}
	tagIDs = normalizeArticleIDs(tagIDs)

	var tagList []models.TagModel
	if err := db.Model(&models.TagModel{}).
		Select("id", "title", "sort").
		Where("id IN ?", tagIDs).
		Order("sort desc, id asc").
		Find(&tagList).Error; err != nil {
		return nil, err
	}
	tagMap := make(map[ctype.ID]models.TagModel, len(tagList))
	for _, tag := range tagList {
		tagMap[tag.ID] = tag
	}

	articleMap := make(map[ctype.ID]*models.ArticleModel, len(articleList))
	for index := range articleList {
		articleList[index].Tags = []models.TagModel{}
		articleMap[articleList[index].ID] = &articleList[index]
	}
	for _, relation := range relationList {
		article, ok := articleMap[relation.ArticleID]
		if !ok {
			continue
		}
		tag, ok := tagMap[relation.TagID]
		if !ok {
			continue
		}
		article.Tags = append(article.Tags, models.TagModel{
			Model: models.Model{ID: tag.ID},
			Title: tag.Title,
			Sort:  tag.Sort,
		})
	}
	for index := range articleList {
		slices.SortFunc(articleList[index].Tags, func(a, b models.TagModel) int {
			if a.Sort != b.Sort {
				if a.Sort > b.Sort {
					return -1
				}
				return 1
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
	}

	return articleList, nil
}

func loadArticleESTopMap(db *gorm.DB, articleIDs []ctype.ID) (map[ctype.ID]articleESTop, error) {
	topMap := make(map[ctype.ID]articleESTop, len(articleIDs))
	if len(articleIDs) == 0 {
		return topMap, nil
	}

	type topRow struct {
		ArticleID ctype.ID
		TopUserID ctype.ID
	}

	var topRows []topRow
	err := db.Model(&models.UserTopArticleModel{}).
		Select("article_id", "user_id AS top_user_id").
		Where("user_top_article_models.article_id IN ?", articleIDs).
		Find(&topRows).Error
	if err != nil {
		return topMap, err
	}
	if len(topRows) == 0 {
		return topMap, nil
	}

	type articleAuthorRow struct {
		ID       ctype.ID
		AuthorID ctype.ID
	}
	var articleRows []articleAuthorRow
	if err := db.Model(&models.ArticleModel{}).
		Select("id", "author_id").
		Where("id IN ?", articleIDs).
		Find(&articleRows).Error; err != nil {
		return topMap, err
	}
	articleAuthorMap := make(map[ctype.ID]ctype.ID, len(articleRows))
	for _, row := range articleRows {
		articleAuthorMap[row.ID] = row.AuthorID
	}

	topUserIDs := make([]ctype.ID, 0, len(topRows))
	for _, row := range topRows {
		topUserIDs = append(topUserIDs, row.TopUserID)
	}
	topUserIDs = normalizeArticleIDs(topUserIDs)

	type userRoleRow struct {
		ID   ctype.ID
		Role enum.RoleType
	}
	var userRows []userRoleRow
	if err := db.Model(&models.UserModel{}).
		Select("id", "role").
		Where("id IN ?", topUserIDs).
		Find(&userRows).Error; err != nil {
		return topMap, err
	}
	roleMap := make(map[ctype.ID]enum.RoleType, len(userRows))
	for _, row := range userRows {
		roleMap[row.ID] = row.Role
	}

	for _, row := range topRows {
		state := topMap[row.ArticleID]
		if roleMap[row.TopUserID] == enum.RoleAdmin {
			state.AdminTop = true
		}
		if row.TopUserID == articleAuthorMap[row.ArticleID] {
			state.AuthorTop = true
		}
		topMap[row.ArticleID] = state
	}
	return topMap, nil
}

func loadCategoryTitleMap(db *gorm.DB, categoryIDs []ctype.ID) (map[ctype.ID]string, error) {
	result := make(map[ctype.ID]string)
	categoryIDs = normalizeArticleIDs(categoryIDs)
	if len(categoryIDs) == 0 {
		return result, nil
	}

	var rows []models.CategoryModel
	if err := db.Model(&models.CategoryModel{}).
		Select("id", "title").
		Where("id IN ?", categoryIDs).
		Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result[row.ID] = row.Title
	}
	return result, nil
}

type authorSnapshot struct {
	Nickname string
	Avatar   string
}

func loadAuthorSnapshotMap(db *gorm.DB, authorIDs []ctype.ID) (map[ctype.ID]authorSnapshot, error) {
	result := make(map[ctype.ID]authorSnapshot)
	authorIDs = normalizeArticleIDs(authorIDs)
	if len(authorIDs) == 0 {
		return result, nil
	}

	type authorRow struct {
		ID       ctype.ID
		Nickname string
		Avatar   string
	}
	var rows []authorRow
	if err := db.Model(&models.UserModel{}).
		Select("id", "nickname", "avatar").
		Where("id IN ?", authorIDs).
		Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result[row.ID] = authorSnapshot{
			Nickname: row.Nickname,
			Avatar:   row.Avatar,
		}
	}
	return result, nil
}
