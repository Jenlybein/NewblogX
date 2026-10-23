package comment_repo

import (
	"time"

	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/models/enum/relationship_enum"
	"myblogx/platform/cachex"
	"myblogx/repository/follow_repo"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type RootCommentItem struct {
	ID           ctype.ID           `json:"id"`
	CreatedAt    time.Time          `json:"created_at"`
	Content      string             `json:"content"`
	UserID       ctype.ID           `json:"user_id"`
	ReplyID      ctype.ID           `json:"reply_id"`
	RootID       ctype.ID           `json:"root_id"`
	DiggCount    int                `json:"digg_count"`
	ReplyCount   int                `json:"reply_count"`
	IsDigg       bool               `json:"is_digg"`
	Relation     int8               `json:"relation"`
	Status       enum.CommentStatus `json:"status"`
	UserNickname string             `json:"user_nickname"`
	UserAvatar   string             `json:"user_avatar"`
}

type ReplyCommentItem struct {
	ID                ctype.ID           `json:"id"`
	CreatedAt         time.Time          `json:"created_at"`
	Content           string             `json:"content"`
	UserID            ctype.ID           `json:"user_id"`
	ReplyID           ctype.ID           `json:"reply_id"`
	DiggCount         int                `json:"digg_count"`
	ReplyCount        int                `json:"reply_count"`
	IsDigg            bool               `json:"is_digg"`
	Relation          int8               `json:"relation"`
	Status            enum.CommentStatus `json:"status"`
	UserNickname      string             `json:"user_nickname"`
	UserAvatar        string             `json:"user_avatar"`
	ReplyUserNickname string             `json:"reply_user_nickname"`
}

type ManageCommentQuery struct {
	Type      int8
	Status    enum.CommentStatus
	ViewerID  ctype.ID
	ArticleID ctype.ID
	UserID    ctype.ID
	PageInfo  common.PageInfo
}

type ManageCommentItem struct {
	ID           ctype.ID `json:"id"`
	CreatedAt    string   `json:"created_at"`
	Content      string   `json:"content"`
	DiggCount    int      `json:"digg_count"`
	ReplyCount   int      `json:"reply_count"`
	UserID       ctype.ID `json:"user_id"`
	UserNickname string   `json:"user_nickname"`
	UserAvatar   string   `json:"user_avatar"`
	Relation     int8     `json:"relation,omitempty"`
	ArticleID    ctype.ID `json:"article_id"`
	ArticleTitle string   `json:"article_title"`
	ArticleCover string   `json:"article_cover"`
}

type QueryService struct {
	DB            *gorm.DB
	CounterReader read_repo.CommentCounterReader
}

func NewQueryService(db *gorm.DB, cacheDeps cachex.Deps) *QueryService {
	return &QueryService{
		DB:            db,
		CounterReader: read_repo.NewCommentCounterReader(cacheDeps),
	}
}

func (s *QueryService) ListPublishedRootComments(articleID ctype.ID, page common.PageInfo, viewerUserID ctype.ID) ([]RootCommentItem, bool, error) {
	limit := page.GetLimit()

	var rows []models.CommentModel
	if err := s.DB.Select(
		"id",
		"created_at",
		"content",
		"user_id",
		"user_nickname",
		"user_avatar",
		"reply_id",
		"root_id",
		"digg_count",
		"reply_count",
		"status",
	).Where(
		"article_id = ? AND status = ? AND reply_id = 0 AND root_id = 0",
		articleID,
		enum.CommentStatusPublished,
	).Order("created_at DESC").
		Limit(limit + 1).
		Offset(page.GetOffsetNoCount()).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items, err := s.buildRootItems(rows, viewerUserID)
	return items, hasMore, err
}

func (s *QueryService) ListPublishedReplyComments(articleID, rootID ctype.ID, page common.PageInfo, viewerUserID ctype.ID) ([]ReplyCommentItem, bool, error) {
	limit := page.GetLimit()

	var rows []models.CommentModel
	if err := s.DB.Select(
		"id",
		"created_at",
		"content",
		"user_id",
		"user_nickname",
		"user_avatar",
		"reply_id",
		"reply_user_id",
		"reply_user_nickname",
		"digg_count",
		"reply_count",
		"status",
	).Where(
		"article_id = ? AND root_id = ? AND status = ?",
		articleID,
		rootID,
		enum.CommentStatusPublished,
	).Order("created_at ASC").
		Limit(limit + 1).
		Offset(page.GetOffsetNoCount()).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items, err := s.buildReplyItems(rows, viewerUserID)
	return items, hasMore, err
}

func (s *QueryService) ListManagedComments(query ManageCommentQuery) ([]ManageCommentItem, int, error) {
	commentQuery := s.DB.Model(&models.CommentModel{})

	switch query.Type {
	case 1:
		articleIDs, err := s.loadAuthorArticleIDs(query.ViewerID, query.ArticleID)
		if err != nil {
			return nil, 0, err
		}
		if len(articleIDs) == 0 {
			return []ManageCommentItem{}, 0, nil
		}
		commentQuery = commentQuery.Where("article_id IN ?", articleIDs).
			Where("status = ?", enum.CommentStatusPublished)
	case 2:
		commentQuery = commentQuery.Where("user_id = ?", query.ViewerID)
	case 3:
	}

	if query.ArticleID != 0 && query.Type != 1 {
		commentQuery = commentQuery.Where("article_id = ?", query.ArticleID)
	}
	if query.UserID != 0 {
		commentQuery = commentQuery.Where("user_id = ?", query.UserID)
	}
	if query.Status != 0 && query.Type != 1 {
		commentQuery = commentQuery.Where("status = ?", query.Status)
	}
	if query.PageInfo.Key != "" {
		commentQuery = commentQuery.Where("content LIKE ?", "%"+query.PageInfo.Key+"%")
	}

	count, err := common.CountQuery(commentQuery)
	if err != nil {
		return nil, 0, err
	}

	var rows []models.CommentModel
	if err = commentQuery.Select(
		"id",
		"created_at",
		"content",
		"user_id",
		"user_nickname",
		"user_avatar",
		"article_id",
		"digg_count",
		"reply_count",
		"status",
	).Order("created_at desc").
		Limit(query.PageInfo.GetLimit()).
		Offset(query.PageInfo.GetOffset(count)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	if err = s.hydrateCommentSnapshots(rows); err != nil {
		return nil, 0, err
	}

	commentIDs := make([]ctype.ID, 0, len(rows))
	userIDs := make([]ctype.ID, 0, len(rows))
	articleIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		commentIDs = append(commentIDs, row.ID)
		userIDs = append(userIDs, row.UserID)
		articleIDs = append(articleIDs, row.ArticleID)
	}

	counters := s.CounterReader.Batch(commentIDs)
	relationMap := make(map[ctype.ID]relationship_enum.Relation)
	if query.Type == 1 {
		relationMap = follow_repo.CalUserRelationshipBatch(s.DB, query.ViewerID, userIDs)
	}
	articleMap, err := read_repo.LoadArticleBaseMap(s.DB, articleIDs)
	if err != nil {
		return nil, 0, err
	}

	items := make([]ManageCommentItem, 0, len(rows))
	for _, row := range rows {
		article := articleMap[row.ArticleID]
		items = append(items, ManageCommentItem{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt.Format("2006-01-02 15:04:05"),
			Content:      row.Content,
			DiggCount:    row.DiggCount + counters.DiggMap[row.ID],
			ReplyCount:   row.ReplyCount + counters.ReplyMap[row.ID],
			UserID:       row.UserID,
			UserNickname: row.UserNickname,
			UserAvatar:   row.UserAvatar,
			Relation:     int8(relationMap[row.UserID]),
			ArticleID:    row.ArticleID,
			ArticleTitle: article.Title,
			ArticleCover: article.Cover,
		})
	}
	return items, count, nil
}

func (s *QueryService) buildRootItems(rows []models.CommentModel, viewerUserID ctype.ID) ([]RootCommentItem, error) {
	if err := s.hydrateCommentSnapshots(rows); err != nil {
		return nil, err
	}

	commentIDs := make([]ctype.ID, 0, len(rows))
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		commentIDs = append(commentIDs, row.ID)
		userIDs = append(userIDs, row.UserID)
	}

	counters := s.CounterReader.Batch(commentIDs)
	isDiggMap := s.buildCommentDiggMap(viewerUserID, commentIDs)
	relationMap := s.buildCommentRelationMap(viewerUserID, userIDs)

	items := make([]RootCommentItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RootCommentItem{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt,
			Content:      row.Content,
			UserID:       row.UserID,
			ReplyID:      row.ReplyId,
			RootID:       row.RootID,
			DiggCount:    row.DiggCount + counters.DiggMap[row.ID],
			ReplyCount:   row.ReplyCount + counters.ReplyMap[row.ID],
			IsDigg:       isDiggMap[row.ID],
			Relation:     int8(relationMap[row.UserID]),
			Status:       row.Status,
			UserNickname: row.UserNickname,
			UserAvatar:   row.UserAvatar,
		})
	}
	return items, nil
}

func (s *QueryService) buildReplyItems(rows []models.CommentModel, viewerUserID ctype.ID) ([]ReplyCommentItem, error) {
	if err := s.hydrateCommentSnapshots(rows); err != nil {
		return nil, err
	}

	commentIDs := make([]ctype.ID, 0, len(rows))
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		commentIDs = append(commentIDs, row.ID)
		userIDs = append(userIDs, row.UserID)
	}

	counters := s.CounterReader.Batch(commentIDs)
	isDiggMap := s.buildCommentDiggMap(viewerUserID, commentIDs)
	relationMap := s.buildCommentRelationMap(viewerUserID, userIDs)

	items := make([]ReplyCommentItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ReplyCommentItem{
			ID:                row.ID,
			CreatedAt:         row.CreatedAt,
			Content:           row.Content,
			UserID:            row.UserID,
			ReplyID:           row.ReplyId,
			DiggCount:         row.DiggCount + counters.DiggMap[row.ID],
			ReplyCount:        row.ReplyCount + counters.ReplyMap[row.ID],
			IsDigg:            isDiggMap[row.ID],
			Relation:          int8(relationMap[row.UserID]),
			Status:            row.Status,
			UserNickname:      row.UserNickname,
			UserAvatar:        row.UserAvatar,
			ReplyUserNickname: row.ReplyUserNickname,
		})
	}
	return items, nil
}

func (s *QueryService) hydrateCommentSnapshots(rows []models.CommentModel) error {
	if len(rows) == 0 {
		return nil
	}

	userIDs := make([]ctype.ID, 0, len(rows))
	replyCommentIDs := make([]ctype.ID, 0, len(rows))
	replyUserIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.UserNickname == "" || row.UserAvatar == "" {
			userIDs = append(userIDs, row.UserID)
		}
		if row.ReplyId != 0 && row.ReplyUserNickname == "" {
			if row.ReplyUserID != 0 {
				replyUserIDs = append(replyUserIDs, row.ReplyUserID)
			} else {
				replyCommentIDs = append(replyCommentIDs, row.ReplyId)
			}
		}
	}

	replyCommentMap := make(map[ctype.ID]models.CommentModel)
	if len(replyCommentIDs) > 0 {
		var replyRows []models.CommentModel
		if err := s.DB.Select("id", "user_id", "user_nickname").
			Where("id IN ?", read_repo.NormalizeIDs(replyCommentIDs)).
			Find(&replyRows).Error; err != nil {
			return err
		}
		for _, replyRow := range replyRows {
			replyCommentMap[replyRow.ID] = replyRow
			if replyRow.UserNickname == "" {
				replyUserIDs = append(replyUserIDs, replyRow.UserID)
			}
		}
	}

	userMap, err := read_repo.LoadUserDisplayMap(s.DB, append(userIDs, replyUserIDs...))
	if err != nil {
		return err
	}

	for i := range rows {
		if rows[i].UserNickname == "" || rows[i].UserAvatar == "" {
			if user, ok := userMap[rows[i].UserID]; ok {
				if rows[i].UserNickname == "" {
					rows[i].UserNickname = user.Nickname
				}
				if rows[i].UserAvatar == "" {
					rows[i].UserAvatar = user.Avatar
				}
			}
		}

		if rows[i].ReplyId == 0 || rows[i].ReplyUserNickname != "" {
			continue
		}
		if rows[i].ReplyUserID != 0 {
			if user, ok := userMap[rows[i].ReplyUserID]; ok {
				rows[i].ReplyUserNickname = user.Nickname
			}
			continue
		}
		if replyRow, ok := replyCommentMap[rows[i].ReplyId]; ok {
			rows[i].ReplyUserID = replyRow.UserID
			if replyRow.UserNickname != "" {
				rows[i].ReplyUserNickname = replyRow.UserNickname
				continue
			}
			if user, ok := userMap[replyRow.UserID]; ok {
				rows[i].ReplyUserNickname = user.Nickname
			}
		}
	}
	return nil
}

func (s *QueryService) loadAuthorArticleIDs(authorID, articleID ctype.ID) ([]ctype.ID, error) {
	if authorID == 0 {
		return nil, nil
	}

	db := s.DB.Model(&models.ArticleModel{}).Select("id").Where("author_id = ?", authorID)
	if articleID != 0 {
		db = db.Where("id = ?", articleID)
	}

	var articleIDs []ctype.ID
	if err := db.Order("id asc").Pluck("id", &articleIDs).Error; err != nil {
		return nil, err
	}
	return articleIDs, nil
}

func (s *QueryService) buildCommentDiggMap(viewerUserID ctype.ID, commentIDs []ctype.ID) map[ctype.ID]bool {
	result := make(map[ctype.ID]bool, len(commentIDs))
	if viewerUserID == 0 || len(commentIDs) == 0 {
		return result
	}

	var diggList []models.CommentDiggModel
	if err := s.DB.Select("comment_id").
		Where("user_id = ? AND comment_id IN ?", viewerUserID, commentIDs).
		Find(&diggList).Error; err != nil {
		return result
	}
	for _, item := range diggList {
		result[item.CommentID] = true
	}
	return result
}

func (s *QueryService) buildCommentRelationMap(viewerUserID ctype.ID, userIDs []ctype.ID) map[ctype.ID]relationship_enum.Relation {
	if viewerUserID == 0 {
		return make(map[ctype.ID]relationship_enum.Relation, len(userIDs))
	}
	return follow_repo.CalUserRelationshipBatch(s.DB, viewerUserID, userIDs)
}
