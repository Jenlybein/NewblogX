package articlesync

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/elastic/go-elasticsearch/v7"
	"gorm.io/gorm"
	"myblogx/internal/platform/es"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/utils/markdown"
)

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

	reqs := make([]*es_service.BulkRequest, 0, len(articleList))
	for _, article := range articleList {
		tags := make([]models.ESTag, 0, len(article.Tags))
		for _, tag := range article.Tags {
			tags = append(tags, models.ESTag{
				ID:    tag.ID,
				Title: tag.Title,
			})
		}
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
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

	reqs := make([]*es_service.BulkRequest, 0, len(articleList))
	for _, article := range articleList {
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
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

	reqs := make([]*es_service.BulkRequest, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		top := topMap[articleID]
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
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

	reqs := make([]*es_service.BulkRequest, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionDelete,
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

	reqs := make([]*es_service.BulkRequest, 0, len(rows))
	for _, row := range rows {
		categoryIDValue := toESNullableID(row.CategoryID)
		var categoryTitle string
		if row.CategoryID != nil {
			categoryTitle = categoryTitleMap[*row.CategoryID]
		}
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
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

	reqs := make([]*es_service.BulkRequest, 0, len(rows))
	for _, row := range rows {
		author := authorMap[row.AuthorID]
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionUpdate,
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

func applyArticlePartialBulkUpdate(db *gorm.DB, client *elasticsearch.Client, reqs []*es_service.BulkRequest, errPrefix string) error {
	if len(reqs) == 0 {
		return nil
	}

	resp := es_service.IndexBulk(client, models.ArticleModel{}.Index(), reqs)
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
