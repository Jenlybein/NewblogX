package chat_repo

import (
	"time"

	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/repository/follow_repo"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type SessionListItem struct {
	SessionID        string     `json:"session_id"`
	ReceiverID       ctype.ID   `json:"receiver_id"`
	ReceiverNickname string     `json:"receiver_nickname"`
	ReceiverAvatar   string     `json:"receiver_avatar"`
	Relation         int8       `json:"relation"`
	LastMsgContent   string     `json:"last_msg_content"`
	LastMsgTime      *time.Time `json:"last_msg_time"`
	UnreadCount      int        `json:"unread_count"`
	IsTop            bool       `json:"is_top"`
	IsMute           bool       `json:"is_mute"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type SessionListQuery struct {
	PageInfo common.PageInfo
	UserID   ctype.ID
	Type     int8
}

type QueryService struct {
	DB *gorm.DB
}

func NewQueryService(db *gorm.DB) *QueryService {
	return &QueryService{DB: db}
}

func (s *QueryService) ListSessions(query SessionListQuery) ([]SessionListItem, int, error) {
	db := s.DB.Model(&models.ChatSessionModel{})
	if query.Type == 2 {
		db = db.Unscoped()
	}
	db = db.Where("user_id = ?", query.UserID)

	count, err := common.CountQuery(db)
	if err != nil {
		return nil, 0, err
	}

	var rows []models.ChatSessionModel
	if err = db.Select(
		"session_id",
		"receiver_id",
		"receiver_nickname",
		"receiver_avatar",
		"last_msg_content",
		"last_msg_time",
		"unread_count",
		"is_top",
		"is_mute",
		"deleted_at",
	).Order("is_top desc, last_msg_time desc, id desc").
		Limit(query.PageInfo.GetLimit()).
		Offset(query.PageInfo.GetOffset(count)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	if err = hydrateChatReceiverSnapshots(s.DB, rows); err != nil {
		return nil, 0, err
	}

	receiverIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		receiverIDs = append(receiverIDs, row.ReceiverID)
	}
	relationMap := follow_repo.CalUserRelationshipBatch(s.DB, query.UserID, receiverIDs)

	list := make([]SessionListItem, 0, len(rows))
	for _, row := range rows {
		item := SessionListItem{
			SessionID:        row.SessionID,
			ReceiverID:       row.ReceiverID,
			ReceiverNickname: row.ReceiverNickname,
			ReceiverAvatar:   row.ReceiverAvatar,
			Relation:         int8(relationMap[row.ReceiverID]),
			LastMsgContent:   row.LastMsgContent,
			LastMsgTime:      row.LastMsgTime,
			UnreadCount:      row.UnreadCount,
			IsTop:            row.IsTop,
			IsMute:           row.IsMute,
		}
		if query.Type == 2 && row.DeletedAt.Valid {
			item.DeletedAt = &row.DeletedAt.Time
		}
		list = append(list, item)
	}
	return list, count, nil
}

func hydrateChatReceiverSnapshots(db *gorm.DB, rows []models.ChatSessionModel) error {
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.ReceiverNickname == "" || row.ReceiverAvatar == "" {
			userIDs = append(userIDs, row.ReceiverID)
		}
	}
	userMap, err := read_repo.LoadUserDisplayMap(db, userIDs)
	if err != nil {
		return err
	}
	for i := range rows {
		user, ok := userMap[rows[i].ReceiverID]
		if !ok {
			continue
		}
		if rows[i].ReceiverNickname == "" {
			rows[i].ReceiverNickname = user.Nickname
		}
		if rows[i].ReceiverAvatar == "" {
			rows[i].ReceiverAvatar = user.Avatar
		}
	}
	return nil
}
