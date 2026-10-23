package favor_repo

import (
	"time"

	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/platform/cachex"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type FavoriteListQuery struct {
	PageInfo  common.PageInfo
	UserID    ctype.ID
	ViewerID  ctype.ID
	ArticleID ctype.ID
	Type      int8
}

type FavoriteListItem struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ID           ctype.ID  `json:"id"`
	UserID       ctype.ID  `json:"user_id"`
	Title        string    `json:"title"`
	Cover        string    `json:"cover"`
	Abstract     string    `json:"abstract"`
	IsDefault    bool      `json:"is_default"`
	ArticleCount int       `json:"article_count"`
	Nickname     string    `json:"nickname,omitempty"`
	Avatar       string    `json:"avatar,omitempty"`
	HasArticle   bool      `json:"has_article"`
}

type FavoriteArticlesQuery struct {
	PageInfo   common.PageInfo
	FavoriteID ctype.ID
}

type FavoriteArticleItem struct {
	FavoritedAt   time.Time          `json:"favorited_at"`
	ArticleID     ctype.ID           `json:"article_id"`
	Title         string             `json:"title"`
	Abstract      string             `json:"abstract"`
	Cover         string             `json:"cover"`
	ViewCount     int                `json:"view_count"`
	DiggCount     int                `json:"digg_count"`
	CommentCount  int                `json:"comment_count"`
	FavorCount    int                `json:"favor_count"`
	UserNickname  string             `json:"user_nickname"`
	UserAvatar    string             `json:"user_avatar"`
	ArticleStatus enum.ArticleStatus `json:"article_status"`
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

func (s *QueryService) ListFavorites(query FavoriteListQuery) ([]FavoriteListItem, int, error) {
	base := s.DB.Model(&models.FavoriteModel{}).Where("user_id = ?", query.UserID)
	if query.PageInfo.Key != "" {
		base = base.Where("title LIKE ?", "%"+query.PageInfo.Key+"%")
	}

	count, err := common.CountQuery(base)
	if err != nil {
		return nil, 0, err
	}

	var rows []models.FavoriteModel
	if err = base.Select(
		"created_at",
		"updated_at",
		"id",
		"user_id",
		"title",
		"cover",
		"abstract",
		"is_default",
		"article_count",
		"owner_nickname",
		"owner_avatar",
	).Order("id desc").
		Limit(query.PageInfo.GetLimit()).
		Offset(query.PageInfo.GetOffset(count)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	if err = hydrateFavoriteOwners(s.DB, rows); err != nil {
		return nil, 0, err
	}
	articleCountMap, err := loadFavoriteArticleCounts(s.DB, rows)
	if err != nil {
		return nil, 0, err
	}
	hasArticleMap, err := s.loadHasArticleMap(query, rows)
	if err != nil {
		return nil, 0, err
	}

	list := make([]FavoriteListItem, 0, len(rows))
	for _, row := range rows {
		articleCount := row.ArticleCount
		if override, ok := articleCountMap[row.ID]; ok {
			articleCount = override
		}

		item := FavoriteListItem{
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
			ID:           row.ID,
			UserID:       row.UserID,
			Title:        row.Title,
			Cover:        row.Cover,
			Abstract:     row.Abstract,
			IsDefault:    row.IsDefault,
			ArticleCount: articleCount,
			HasArticle:   hasArticleMap[row.ID],
		}
		if query.Type == 3 {
			item.Nickname = row.OwnerNickname
			item.Avatar = row.OwnerAvatar
		}
		list = append(list, item)
	}
	return list, count, nil
}

func (s *QueryService) ListFavoriteArticles(query FavoriteArticlesQuery, orderMap map[string]string) ([]FavoriteArticleItem, bool, error) {
	base := s.DB.Model(&models.UserArticleFavorModel{}).
		Where("favor_id = ?", query.FavoriteID)

	if query.PageInfo.Key != "" {
		var articleIDs []ctype.ID
		if err := s.DB.Model(&models.ArticleModel{}).
			Select("id").
			Where("publish_status = ? AND title LIKE ?", enum.ArticleStatusPublished, "%"+query.PageInfo.Key+"%").
			Pluck("id", &articleIDs).Error; err != nil {
			return nil, false, err
		}
		if len(articleIDs) == 0 {
			return []FavoriteArticleItem{}, false, nil
		}
		base = base.Where("article_id IN ?", articleIDs)
	}

	order, err := common.ResolveOrder(query.PageInfo.Order, orderMap, "created_at desc")
	if err != nil {
		return nil, false, err
	}

	limit := query.PageInfo.GetLimit()
	offset := query.PageInfo.GetOffsetNoCount()
	var rows []models.UserArticleFavorModel
	if err = base.Select(
		"id",
		"created_at",
		"article_id",
		"article_title",
		"article_abstract",
		"article_cover",
		"article_status",
		"article_author_id",
		"article_author_nickname",
		"article_author_avatar",
	).Order(order).
		Limit(limit + 1).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	if err = hydrateFavoriteArticleSnapshots(s.DB, rows); err != nil {
		return nil, false, err
	}

	articleIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		articleIDs = append(articleIDs, row.ArticleID)
	}
	articleBaseMap, err := read_repo.LoadArticleBaseMap(s.DB, articleIDs)
	if err != nil {
		return nil, false, err
	}
	authorIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.ArticleAuthorID != 0 {
			authorIDs = append(authorIDs, row.ArticleAuthorID)
			continue
		}
		if articleBase, ok := articleBaseMap[row.ArticleID]; ok {
			authorIDs = append(authorIDs, articleBase.AuthorID)
		}
	}
	authorMap, err := read_repo.LoadUserDisplayMap(s.DB, authorIDs)
	if err != nil {
		return nil, false, err
	}
	counters := s.ArticleReader.Batch(articleIDs)

	list := make([]FavoriteArticleItem, 0, len(rows))
	for _, row := range rows {
		articleBase := articleBaseMap[row.ArticleID]
		if articleBase.PublishStatus != enum.ArticleStatusPublished {
			continue
		}

		title := row.ArticleTitle
		abstract := row.ArticleAbstract
		cover := row.ArticleCover
		authorNickname := row.ArticleAuthorNickname
		authorAvatar := row.ArticleAuthorAvatar
		if title == "" {
			title = articleBase.Title
		}
		if abstract == "" {
			abstract = articleBase.Abstract
		}
		if cover == "" {
			cover = articleBase.Cover
		}
		if authorNickname == "" || authorAvatar == "" {
			userMap := authorMap
			if authorNickname == "" {
				authorNickname = userMap[articleBase.AuthorID].Nickname
			}
			if authorAvatar == "" {
				authorAvatar = userMap[articleBase.AuthorID].Avatar
			}
		}

		list = append(list, FavoriteArticleItem{
			FavoritedAt:   row.CreatedAt,
			ArticleID:     row.ArticleID,
			Title:         title,
			Abstract:      abstract,
			Cover:         cover,
			ViewCount:     articleBase.ViewCount + counters.ViewMap[row.ArticleID],
			DiggCount:     articleBase.DiggCount + counters.DiggMap[row.ArticleID],
			CommentCount:  articleBase.CommentCount + counters.CommentMap[row.ArticleID],
			FavorCount:    articleBase.FavorCount + counters.FavorMap[row.ArticleID],
			UserNickname:  authorNickname,
			UserAvatar:    authorAvatar,
			ArticleStatus: articleBase.PublishStatus,
		})
	}
	return list, hasMore, nil
}

func hydrateFavoriteOwners(db *gorm.DB, rows []models.FavoriteModel) error {
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.OwnerNickname == "" || row.OwnerAvatar == "" {
			userIDs = append(userIDs, row.UserID)
		}
	}
	userMap, err := read_repo.LoadUserDisplayMap(db, userIDs)
	if err != nil {
		return err
	}
	for i := range rows {
		user, ok := userMap[rows[i].UserID]
		if !ok {
			continue
		}
		if rows[i].OwnerNickname == "" {
			rows[i].OwnerNickname = user.Nickname
		}
		if rows[i].OwnerAvatar == "" {
			rows[i].OwnerAvatar = user.Avatar
		}
	}
	return nil
}

func loadFavoriteArticleCounts(db *gorm.DB, rows []models.FavoriteModel) (map[ctype.ID]int, error) {
	result := make(map[ctype.ID]int)
	favoriteIDs := make([]ctype.ID, 0, len(rows))
	needFallback := false
	for _, row := range rows {
		favoriteIDs = append(favoriteIDs, row.ID)
		if row.ArticleCount == 0 {
			needFallback = true
		}
	}
	if !needFallback || len(favoriteIDs) == 0 {
		return result, nil
	}

	type countRow struct {
		FavorID      ctype.ID
		ArticleCount int
	}
	var countRows []countRow
	if err := db.Model(&models.UserArticleFavorModel{}).
		Select("favor_id, COUNT(*) AS article_count").
		Where("favor_id IN ?", favoriteIDs).
		Group("favor_id").
		Scan(&countRows).Error; err != nil {
		return result, err
	}
	for _, row := range countRows {
		result[row.FavorID] = row.ArticleCount
	}
	return result, nil
}

func hydrateFavoriteArticleSnapshots(db *gorm.DB, rows []models.UserArticleFavorModel) error {
	articleIDs := make([]ctype.ID, 0, len(rows))
	authorIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.ArticleTitle == "" || row.ArticleAbstract == "" || row.ArticleCover == "" || row.ArticleAuthorID == 0 {
			articleIDs = append(articleIDs, row.ArticleID)
			continue
		}
		if row.ArticleAuthorNickname == "" || row.ArticleAuthorAvatar == "" {
			authorIDs = append(authorIDs, row.ArticleAuthorID)
		}
	}

	articleMap, err := read_repo.LoadArticleBaseMap(db, articleIDs)
	if err != nil {
		return err
	}
	for _, article := range articleMap {
		authorIDs = append(authorIDs, article.AuthorID)
	}
	userMap, err := read_repo.LoadUserDisplayMap(db, authorIDs)
	if err != nil {
		return err
	}

	for i := range rows {
		article, ok := articleMap[rows[i].ArticleID]
		if ok {
			if rows[i].ArticleTitle == "" {
				rows[i].ArticleTitle = article.Title
			}
			if rows[i].ArticleAbstract == "" {
				rows[i].ArticleAbstract = article.Abstract
			}
			if rows[i].ArticleCover == "" {
				rows[i].ArticleCover = article.Cover
			}
			if rows[i].ArticleAuthorID == 0 {
				rows[i].ArticleAuthorID = article.AuthorID
			}
		}
		if user, ok := userMap[rows[i].ArticleAuthorID]; ok {
			if rows[i].ArticleAuthorNickname == "" {
				rows[i].ArticleAuthorNickname = user.Nickname
			}
			if rows[i].ArticleAuthorAvatar == "" {
				rows[i].ArticleAuthorAvatar = user.Avatar
			}
		}
	}
	return nil
}

func (s *QueryService) loadHasArticleMap(query FavoriteListQuery, rows []models.FavoriteModel) (map[ctype.ID]bool, error) {
	result := make(map[ctype.ID]bool, len(rows))
	if query.Type != 1 || query.ArticleID == 0 || query.ViewerID == 0 || len(rows) == 0 {
		return result, nil
	}

	favoriteIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		favoriteIDs = append(favoriteIDs, row.ID)
	}

	var relationList []models.UserArticleFavorModel
	if err := s.DB.Select("favor_id").
		Where("user_id = ? AND article_id = ? AND favor_id IN ?", query.ViewerID, query.ArticleID, favoriteIDs).
		Find(&relationList).Error; err != nil {
		return result, err
	}

	for _, relation := range relationList {
		result[relation.FavorID] = true
	}
	return result, nil
}
