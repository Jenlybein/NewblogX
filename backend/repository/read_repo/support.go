package read_repo

import (
	"sort"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"

	"gorm.io/gorm"
)

type UserDisplay struct {
	ID       ctype.ID
	Nickname string
	Avatar   string
	Abstract string
	Role     enum.RoleType
}

type ArticleBase struct {
	ID             ctype.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Title          string
	Abstract       string
	Cover          string
	AuthorID       ctype.ID
	CategoryID     *ctype.ID
	ViewCount      int
	DiggCount      int
	CommentCount   int
	FavorCount     int
	CommentsToggle bool
	PublishStatus  enum.ArticleStatus
}

func NormalizeIDs(ids []ctype.ID) []ctype.ID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[ctype.ID]struct{}, len(ids))
	result := make([]ctype.ID, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func LoadUserDisplayMap(db *gorm.DB, userIDs []ctype.ID) (map[ctype.ID]UserDisplay, error) {
	result := make(map[ctype.ID]UserDisplay)
	userIDs = NormalizeIDs(userIDs)
	if len(userIDs) == 0 {
		return result, nil
	}

	var rows []models.UserModel
	if err := db.Select("id", "nickname", "avatar", "abstract", "role").
		Where("id IN ?", userIDs).
		Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result[row.ID] = UserDisplay{
			ID:       row.ID,
			Nickname: row.Nickname,
			Avatar:   row.Avatar,
			Abstract: row.Abstract,
			Role:     row.Role,
		}
	}
	return result, nil
}

func LoadArticleBaseMap(db *gorm.DB, articleIDs []ctype.ID) (map[ctype.ID]ArticleBase, error) {
	result := make(map[ctype.ID]ArticleBase)
	articleIDs = NormalizeIDs(articleIDs)
	if len(articleIDs) == 0 {
		return result, nil
	}

	var rows []models.ArticleModel
	if err := db.Select(
		"id",
		"created_at",
		"updated_at",
		"title",
		"abstract",
		"cover",
		"author_id",
		"category_id",
		"view_count",
		"digg_count",
		"comment_count",
		"favor_count",
		"comments_toggle",
		"publish_status",
	).Where("id IN ?", articleIDs).Find(&rows).Error; err != nil {
		return result, err
	}

	for _, row := range rows {
		result[row.ID] = ArticleBase{
			ID:             row.ID,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
			Title:          row.Title,
			Abstract:       row.Abstract,
			Cover:          row.Cover,
			AuthorID:       row.AuthorID,
			CategoryID:     row.CategoryID,
			ViewCount:      row.ViewCount,
			DiggCount:      row.DiggCount,
			CommentCount:   row.CommentCount,
			FavorCount:     row.FavorCount,
			CommentsToggle: row.CommentsToggle,
			PublishStatus:  row.PublishStatus,
		}
	}
	return result, nil
}

func LoadCategoryTitleMap(db *gorm.DB, categoryIDs []ctype.ID) (map[ctype.ID]string, error) {
	result := make(map[ctype.ID]string)
	categoryIDs = NormalizeIDs(categoryIDs)
	if len(categoryIDs) == 0 {
		return result, nil
	}

	var rows []models.CategoryModel
	if err := db.Select("id", "title").Where("id IN ?", categoryIDs).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		result[row.ID] = row.Title
	}
	return result, nil
}

func LoadArticleTagTitlesMap(db *gorm.DB, articleIDs []ctype.ID) (map[ctype.ID][]string, error) {
	result := make(map[ctype.ID][]string)
	articleIDs = NormalizeIDs(articleIDs)
	if len(articleIDs) == 0 {
		return result, nil
	}

	var relations []models.ArticleTagModel
	if err := db.Select("article_id", "tag_id").
		Where("article_id IN ?", articleIDs).
		Order("article_id asc, tag_id asc").
		Find(&relations).Error; err != nil {
		return result, err
	}
	if len(relations) == 0 {
		return result, nil
	}

	tagIDs := make([]ctype.ID, 0, len(relations))
	for _, relation := range relations {
		tagIDs = append(tagIDs, relation.TagID)
	}
	tagIDs = NormalizeIDs(tagIDs)

	var tags []models.TagModel
	if err := db.Select("id", "title", "sort").
		Where("id IN ?", tagIDs).
		Order("sort desc, id asc").
		Find(&tags).Error; err != nil {
		return result, err
	}
	tagTitleMap := make(map[ctype.ID]string, len(tags))
	for _, tag := range tags {
		tagTitleMap[tag.ID] = tag.Title
	}

	for _, relation := range relations {
		title, ok := tagTitleMap[relation.TagID]
		if !ok {
			continue
		}
		result[relation.ArticleID] = append(result[relation.ArticleID], title)
	}
	return result, nil
}
