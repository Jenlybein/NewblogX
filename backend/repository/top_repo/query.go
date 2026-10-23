package top_repo

import (
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/platform/cachex"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type ArticleTopListItem struct {
	ID             ctype.ID           `json:"id"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Title          string             `json:"title"`
	Abstract       string             `json:"abstract"`
	Cover          string             `json:"cover"`
	ViewCount      int                `json:"view_count"`
	DiggCount      int                `json:"digg_count"`
	CommentCount   int                `json:"comment_count"`
	FavorCount     int                `json:"favor_count"`
	CommentsToggle bool               `json:"comments_toggle"`
	Status         enum.ArticleStatus `json:"status"`
	Tags           []string           `json:"tags"`
	UserTop        bool               `json:"user_top"`
	AdminTop       bool               `json:"admin_top"`
	CategoryTitle  string             `json:"category_title"`
	UserNickname   string             `json:"user_nickname"`
	UserAvatar     string             `json:"user_avatar"`
}

type QueryService struct {
	DB            *gorm.DB
	ArticleReader read_repo.ArticleCounterReader
}

func NewQueryService(db *gorm.DB, cacheDeps cachex.Deps) *QueryService {
	return &QueryService{
		DB:            db,
		ArticleReader: read_repo.NewArticleCounterReader(cacheDeps),
	}
}

func (s *QueryService) ListArticles(topType int, userID ctype.ID) ([]ArticleTopListItem, error) {
	rows, err := s.loadTopRows(s.DB, topType, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ArticleTopListItem{}, nil
	}

	articleIDs := make([]ctype.ID, 0, len(rows))
	topUserIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		articleIDs = append(articleIDs, row.ArticleID)
		topUserIDs = append(topUserIDs, row.UserID)
	}
	articleIDs = read_repo.NormalizeIDs(articleIDs)
	articleBaseMap, err := read_repo.LoadArticleBaseMap(s.DB, articleIDs)
	if err != nil {
		return nil, err
	}

	allTopRows, err := s.loadAllTopRows(s.DB, articleIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range allTopRows {
		topUserIDs = append(topUserIDs, row.UserID)
	}
	userMap, err := read_repo.LoadUserDisplayMap(s.DB, topUserIDs)
	if err != nil {
		return nil, err
	}

	categoryIDs := make([]ctype.ID, 0, len(articleBaseMap))
	authorIDs := make([]ctype.ID, 0, len(articleBaseMap))
	for _, article := range articleBaseMap {
		if article.CategoryID != nil {
			categoryIDs = append(categoryIDs, *article.CategoryID)
		}
		authorIDs = append(authorIDs, article.AuthorID)
	}
	authorMap, err := read_repo.LoadUserDisplayMap(s.DB, authorIDs)
	if err != nil {
		return nil, err
	}
	categoryMap, err := read_repo.LoadCategoryTitleMap(s.DB, categoryIDs)
	if err != nil {
		return nil, err
	}
	tagMap, err := read_repo.LoadArticleTagTitlesMap(s.DB, articleIDs)
	if err != nil {
		return nil, err
	}
	topStateMap := buildTopStateMap(allTopRows, articleBaseMap, userMap)
	counters := s.ArticleReader.Batch(articleIDs)

	list := make([]ArticleTopListItem, 0, len(rows))
	for _, row := range rows {
		article, ok := articleBaseMap[row.ArticleID]
		if !ok || article.PublishStatus != enum.ArticleStatusPublished {
			continue
		}
		if topType == 1 && article.AuthorID != userID {
			continue
		}
		topState := topStateMap[row.ArticleID]
		author := authorMap[article.AuthorID]
		item := ArticleTopListItem{
			ID:             article.ID,
			CreatedAt:      article.CreatedAt,
			UpdatedAt:      article.UpdatedAt,
			Title:          article.Title,
			Abstract:       article.Abstract,
			Cover:          article.Cover,
			ViewCount:      article.ViewCount + counters.ViewMap[article.ID],
			DiggCount:      article.DiggCount + counters.DiggMap[article.ID],
			CommentCount:   article.CommentCount + counters.CommentMap[article.ID],
			FavorCount:     article.FavorCount + counters.FavorMap[article.ID],
			CommentsToggle: article.CommentsToggle,
			Status:         article.PublishStatus,
			Tags:           tagMap[article.ID],
			UserTop:        topState.UserTop,
			AdminTop:       topState.AdminTop,
			UserNickname:   author.Nickname,
			UserAvatar:     author.Avatar,
		}
		if article.CategoryID != nil {
			item.CategoryTitle = categoryMap[*article.CategoryID]
		}
		list = append(list, item)
	}
	return list, nil
}

func (s *QueryService) loadTopRows(db *gorm.DB, topType int, userID ctype.ID) ([]models.UserTopArticleModel, error) {
	var rows []models.UserTopArticleModel
	query := db.Model(&models.UserTopArticleModel{}).Select("article_id", "user_id", "created_at", "updated_at")
	switch topType {
	case 1:
		query = query.Where("user_id = ?", userID)
	case 2:
		adminIDs, err := s.loadAdminUserIDs(db)
		if err != nil {
			return nil, err
		}
		if len(adminIDs) == 0 {
			return nil, nil
		}
		query = query.Where("user_id IN ?", adminIDs)
	}
	if err := query.Order("created_at desc, id desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := make(map[ctype.ID]struct{}, len(rows))
	result := make([]models.UserTopArticleModel, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.ArticleID]; ok {
			continue
		}
		seen[row.ArticleID] = struct{}{}
		result = append(result, row)
	}
	return result, nil
}

func (s *QueryService) loadAllTopRows(db *gorm.DB, articleIDs []ctype.ID) ([]models.UserTopArticleModel, error) {
	var rows []models.UserTopArticleModel
	if len(articleIDs) == 0 {
		return rows, nil
	}
	err := db.Model(&models.UserTopArticleModel{}).
		Select("article_id", "user_id").
		Where("article_id IN ?", articleIDs).
		Find(&rows).Error
	return rows, err
}

func (s *QueryService) loadAdminUserIDs(db *gorm.DB) ([]ctype.ID, error) {
	var userIDs []ctype.ID
	if err := db.Model(&models.UserModel{}).
		Select("id").
		Where("role = ?", enum.RoleAdmin).
		Pluck("id", &userIDs).Error; err != nil {
		return nil, err
	}
	return userIDs, nil
}

type topState struct {
	UserTop  bool
	AdminTop bool
}

func buildTopStateMap(rows []models.UserTopArticleModel, articleMap map[ctype.ID]read_repo.ArticleBase, userMap map[ctype.ID]read_repo.UserDisplay) map[ctype.ID]topState {
	result := make(map[ctype.ID]topState, len(articleMap))
	for _, row := range rows {
		article, ok := articleMap[row.ArticleID]
		if !ok {
			continue
		}
		state := result[row.ArticleID]
		if userMap[row.UserID].Role == enum.RoleAdmin {
			state.AdminTop = true
		}
		if row.UserID == article.AuthorID {
			state.UserTop = true
		}
		result[row.ArticleID] = state
	}
	return result
}
