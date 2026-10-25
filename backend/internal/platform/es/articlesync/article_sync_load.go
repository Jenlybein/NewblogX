package articlesync

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/es"
	"myblogx/models"
)

func collectArticleBulkFailures(data any, reqs []*es_service.BulkRequest, maxItems int) ([]articleBulkFailure, []ctype.ID) {
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

			if action == es_service.ActionDelete && errType == "document_missing_exception" {
				continue
			}
			if action == es_service.ActionUpdate && errType == "document_missing_exception" {
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

func loadArticleProjectionOptions(db *gorm.DB, snapshots []ArticleRowSnapshot) (articleProjectionOptions, error) {
	deps := articleProjectionOptions{
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
