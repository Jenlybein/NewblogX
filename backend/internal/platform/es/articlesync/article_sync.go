package articlesync

import (
	"fmt"
	"strconv"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/es"
	"myblogx/utils/markdown"
)

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
	// DeletedAt 是 GORM 软删时间。非空表示这一行已被删除，文档里要写软删标记。
	//
	// 全量重建会把已软删的行一起索引（索引是库的镜像，读侧再按标记过滤），
	// 所以快照必须带上它 —— 否则重建出来的文档永远显示 is_deleted=false，
	// 兜底过滤等于没有。
	DeletedAt *time.Time
}

type articleProjectionOptions struct {
	CategoryTitleMap map[ctype.ID]string
	AuthorMap        map[ctype.ID]authorSnapshot
	ArticleTagsMap   map[ctype.ID][]ESTag
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
	default:
		return nil
	}
}

// UpdateESDocsByArticleDeltas 按 article_models 的“变更列”做局部更新。
// 说明：
//  1. 仅更新真正变更的展示字段，避免每次 update 都整篇重建 ES 文档。
//  2. 对 category/author 这类快照字段，会批量查询必要信息后再补齐。
//  3. 当 author_id 变更时，会连同 author_is_admin（作者角色投影）一起重算，
//     否则换作者后的文档会带着上一个作者的角色。
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
	for _, delta := range deltas {
		if rawCategoryID, ok := delta.Changed["category_id"]; ok {
			if categoryID, ok := scanNullableIDValue(rawCategoryID); ok && categoryID != nil {
				categoryIDs = append(categoryIDs, *categoryID)
			}
		}
		if rawAuthorID, ok := delta.Changed["author_id"]; ok {
			if authorID, ok := scanIDValue(rawAuthorID); ok && !authorID.IsZero() {
				authorIDs = append(authorIDs, authorID)
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

	reqs := make([]*es_service.BulkRequest, 0, len(deltas))
	for _, delta := range deltas {
		// 软删在库里的形态是 deleted_at 的一次 UPDATE（行还在，只是打了标记）。
		// 它不在下面的可投影列里：若照旧走"变更列 → 局部更新"，data 会是空的，
		// 这次更新对索引零写入，文档就永远留着。命中软删标记时直接删文档，
		// 与其他动作共用同一条 bulk 路径（重试、缺失文档判定都不用另写一套）。
		if articleSoftDeleted(delta.Changed) {
			reqs = append(reqs, &es_service.BulkRequest{
				Action: es_service.ActionDelete,
				ID:     strconv.FormatUint(delta.ArticleID.Uint64(), 10),
			})
			continue
		}

		data := make(map[string]any)
		restored := articleRestored(delta.Changed)
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
					data["author_is_admin"] = author.IsAdmin
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

		if restored {
			// 恢复（deleted_at 改回 NULL）不在可投影列里：旧实现会得到空 data
			// 然后 continue —— 这次恢复对索引零写入，标记永远停在 true。
			// 文档可能早已被 #196 删掉（update 404 → 走既有的缺失文档回源重建），
			// 也可能还带着 is_deleted=true；两种情况下都要把标记清回 false，
			// 否则这篇恢复的文章在任何列表里都出不来。
			data[SoftDeleteMarkerField] = false
		} else if len(data) == 0 {
			continue
		}
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
			ID:     strconv.FormatUint(delta.ArticleID.Uint64(), 10),
			Data:   data,
		})
	}

	return applyArticlePartialBulkUpdate(db, client, reqs, "按 article_models 差异更新 ES 文档失败")
}

// articleSoftDeleted 判断一份"变更列快照"是否表示文章被软删。
//
// 只看 deleted_at 的**变更后值**非空：从时间改回 NULL（恢复）不是删除，
// 这里不猜恢复语义 —— 恢复需要重建文档，不在本次修复范围。
func articleSoftDeleted(changed map[string]any) bool {
	raw, ok := changed["deleted_at"]
	if !ok {
		return false
	}
	value, ok := scanTimeValue(raw)
	return ok && !value.IsZero()
}

// articleRestored 判断一份"变更列快照"是否表示文章被恢复（deleted_at 改回 NULL）。
//
// 与 articleSoftDeleted 互斥：软删是时间列有值，恢复是它被清空。
// 只有"恢复"需要清软删标记；软删本身由 #196 直接删文档，不需要再写标记。
func articleRestored(changed map[string]any) bool {
	raw, ok := changed["deleted_at"]
	if !ok {
		return false
	}
	if raw == nil {
		return true
	}
	value, ok := scanTimeValue(raw)
	return ok && value.IsZero()
}

func buildArticleESDocumentFromSnapshot(snapshot ArticleRowSnapshot, deps articleProjectionOptions) map[string]any {
	categoryIDValue := toESNullableID(snapshot.CategoryID)
	var categoryTitle string
	if snapshot.CategoryID != nil {
		categoryTitle = deps.CategoryTitleMap[*snapshot.CategoryID]
	}
	author := deps.AuthorMap[snapshot.AuthorID]
	tags := deps.ArticleTagsMap[snapshot.ID]
	if tags == nil {
		tags = []ESTag{}
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
		// 作者角色投影：公开查询在“显示非管理员文章”关掉时按它过滤（ADR-0017 决策 4）。
		"author_is_admin":   author.IsAdmin,
		"view_count":        snapshot.ViewCount,
		"digg_count":        snapshot.DiggCount,
		"comment_count":     snapshot.CommentCount,
		"favor_count":       snapshot.FavorCount,
		"publish_status":    snapshot.PublishStatus,
		"visibility_status": snapshot.VisibilityStatus,
		"comments_toggle":   snapshot.CommentsToggle,
		"tags":              tags,
		// 软删标记：全量重建会把已软删的行一起索引，读侧靠它把这些文档挡在
		// 所有列表之外（internal/search/app 的兜底过滤）。未删除也显式写 false。
		SoftDeleteMarkerField: snapshot.DeletedAt != nil,
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
// 这里会从数据库重新读取文章、标签与作者/分类快照，再统一索引到 ES。
// 写入侧不再同步任何置顶派生字段（ADR-0017 决策 6），作者置顶只留在 MySQL。
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

	deps, err := loadArticleProjectionOptions(db, snapshots)
	if err != nil {
		return err
	}

	reqs := make([]*es_service.BulkRequest, 0, len(snapshots))
	for _, snapshot := range snapshots {
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionIndex,
			ID:     strconv.FormatUint(snapshot.ID.Uint64(), 10),
			Data:   buildArticleESDocumentFromSnapshot(snapshot, deps),
		})
	}

	return bulkWriteError(bulkWriteWithRetry(client, reqs, 3, defaultBulkRetryPolicy), errPrefix)
}
