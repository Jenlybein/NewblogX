package follow_repo

import (
	"time"

	"myblogx/common"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/relationship_enum"
	"myblogx/repository/read_repo"

	"gorm.io/gorm"
)

type FollowListItem struct {
	FollowedUserID   ctype.ID  `json:"followed_user_id"`
	FollowedNickname string    `json:"followed_nickname"`
	FollowedAvatar   string    `json:"followed_avatar"`
	FollowedAbstract string    `json:"followed_abstract"`
	FollowTime       time.Time `json:"follow_time"`
	Relation         int8      `json:"relation"`
}

type FansListItem struct {
	FansUserID   ctype.ID  `json:"fans_user_id"`
	FansNickname string    `json:"fans_nickname"`
	FansAvatar   string    `json:"fans_avatar"`
	FansAbstract string    `json:"fans_abstract"`
	FollowTime   time.Time `json:"follow_time"`
	Relation     int8      `json:"relation"`
}

type QueryService struct {
	DB *gorm.DB
}

func NewQueryService(db *gorm.DB) *QueryService {
	return &QueryService{DB: db}
}

func (s *QueryService) ListFollowing(ownerUserID, viewerUserID, followedUserID ctype.ID, page common.PageInfo) ([]FollowListItem, bool, error) {
	query := s.DB.Model(&models.UserFollowModel{}).Where("fans_user_id = ?", ownerUserID)
	if followedUserID != 0 {
		query = query.Where("followed_user_id = ?", followedUserID)
	}

	limit := page.GetLimit()
	offset := page.GetOffsetNoCount()
	var rows []models.UserFollowModel
	if err := query.Select(
		"created_at",
		"followed_user_id",
		"followed_nickname",
		"followed_avatar",
		"followed_abstract",
	).Order("created_at asc, id asc").
		Limit(limit + 1).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	if err := hydrateFollowedSnapshots(s.DB, rows); err != nil {
		return nil, false, err
	}

	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.FollowedUserID)
	}
	relationMap := CalUserRelationshipBatch(s.DB, viewerUserID, userIDs)

	list := make([]FollowListItem, 0, len(rows))
	for _, row := range rows {
		relation := relationMap[row.FollowedUserID]
		if viewerUserID == 0 {
			relation = relationship_enum.RelationStranger
		}
		list = append(list, FollowListItem{
			FollowedUserID:   row.FollowedUserID,
			FollowedNickname: row.FollowedNickname,
			FollowedAvatar:   row.FollowedAvatar,
			FollowedAbstract: row.FollowedAbstract,
			FollowTime:       row.CreatedAt,
			Relation:         int8(relation),
		})
	}
	return list, hasMore, nil
}

func (s *QueryService) ListFans(ownerUserID, viewerUserID, fansUserID ctype.ID, page common.PageInfo) ([]FansListItem, bool, error) {
	query := s.DB.Model(&models.UserFollowModel{}).Where("followed_user_id = ?", ownerUserID)
	if fansUserID != 0 {
		query = query.Where("fans_user_id = ?", fansUserID)
	}

	limit := page.GetLimit()
	offset := page.GetOffsetNoCount()
	var rows []models.UserFollowModel
	if err := query.Select(
		"created_at",
		"fans_user_id",
		"fans_nickname",
		"fans_avatar",
		"fans_abstract",
	).Order("created_at asc, id asc").
		Limit(limit + 1).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	if err := hydrateFansSnapshots(s.DB, rows); err != nil {
		return nil, false, err
	}

	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.FansUserID)
	}
	relationMap := CalUserRelationshipBatch(s.DB, viewerUserID, userIDs)

	list := make([]FansListItem, 0, len(rows))
	for _, row := range rows {
		relation := relationMap[row.FansUserID]
		if viewerUserID == 0 {
			relation = relationship_enum.RelationStranger
		}
		list = append(list, FansListItem{
			FansUserID:   row.FansUserID,
			FansNickname: row.FansNickname,
			FansAvatar:   row.FansAvatar,
			FansAbstract: row.FansAbstract,
			FollowTime:   row.CreatedAt,
			Relation:     int8(relation),
		})
	}
	return list, hasMore, nil
}

func hydrateFollowedSnapshots(db *gorm.DB, rows []models.UserFollowModel) error {
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.FollowedNickname == "" || row.FollowedAvatar == "" || row.FollowedAbstract == "" {
			userIDs = append(userIDs, row.FollowedUserID)
		}
	}
	userMap, err := read_repo.LoadUserDisplayMap(db, userIDs)
	if err != nil {
		return err
	}
	for i := range rows {
		user, ok := userMap[rows[i].FollowedUserID]
		if !ok {
			continue
		}
		if rows[i].FollowedNickname == "" {
			rows[i].FollowedNickname = user.Nickname
		}
		if rows[i].FollowedAvatar == "" {
			rows[i].FollowedAvatar = user.Avatar
		}
		if rows[i].FollowedAbstract == "" {
			rows[i].FollowedAbstract = user.Abstract
		}
	}
	return nil
}

func hydrateFansSnapshots(db *gorm.DB, rows []models.UserFollowModel) error {
	userIDs := make([]ctype.ID, 0, len(rows))
	for _, row := range rows {
		if row.FansNickname == "" || row.FansAvatar == "" || row.FansAbstract == "" {
			userIDs = append(userIDs, row.FansUserID)
		}
	}
	userMap, err := read_repo.LoadUserDisplayMap(db, userIDs)
	if err != nil {
		return err
	}
	for i := range rows {
		user, ok := userMap[rows[i].FansUserID]
		if !ok {
			continue
		}
		if rows[i].FansNickname == "" {
			rows[i].FansNickname = user.Nickname
		}
		if rows[i].FansAvatar == "" {
			rows[i].FansAvatar = user.Avatar
		}
		if rows[i].FansAbstract == "" {
			rows[i].FansAbstract = user.Abstract
		}
	}
	return nil
}
