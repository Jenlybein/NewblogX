package global_notif_api

import (
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/global_notif_enum"
	"time"

	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type GlobalNotifApi struct {
	App Deps
}

func New(deps Deps) GlobalNotifApi {
	return GlobalNotifApi{App: deps}
}

type UserGlobalNotifState struct {
	User             models.UserModel
	UserNotifMap     map[ctype.ID]models.UserGlobalNotifModel
	DeletedMsgIDList []ctype.ID
}

func BuildUserVisibleGlobalNotifQuery(db *gorm.DB, user models.UserModel) *gorm.DB {
	now := time.Now()
	return db.
		Where("expire_time > ?", now).
		Where(db.Where("user_visible_rule = ?", global_notif_enum.UserVisibleAllUsers).
			Or(db.Where(
				"user_visible_rule = ? AND created_at >= ? AND expire_time >= ?",
				global_notif_enum.UserVisibleRegisteredUsers,
				user.CreatedAt,
				user.CreatedAt,
			)).
			Or(db.Where(
				"user_visible_rule = ? AND created_at < ? AND expire_time >= ?",
				global_notif_enum.UserVisibleNewUsers,
				user.CreatedAt,
				user.CreatedAt,
			)))
}

func LoadUserGlobalNotifState(db *gorm.DB, userID ctype.ID, msgIDList []ctype.ID) (state UserGlobalNotifState, err error) {
	if err = db.Take(&state.User, userID).Error; err != nil {
		return state, err
	}

	query := db.Unscoped().Where("user_id = ?", userID)
	if len(msgIDList) > 0 {
		query = query.Where("msg_id IN ?", msgIDList)
	}

	var userNotifList []models.UserGlobalNotifModel
	if err = query.Find(&userNotifList).Error; err != nil {
		return state, err
	}

	state.UserNotifMap = make(map[ctype.ID]models.UserGlobalNotifModel, len(userNotifList))
	state.DeletedMsgIDList = make([]ctype.ID, 0)
	for _, item := range userNotifList {
		state.UserNotifMap[item.MsgID] = item
		if item.DeletedAt.Valid {
			state.DeletedMsgIDList = append(state.DeletedMsgIDList, item.MsgID)
		}
	}
	return state, nil
}

func BuildUserVisibleGlobalNotifListQuery(db *gorm.DB, state UserGlobalNotifState) *gorm.DB {
	query := db.Where(BuildUserVisibleGlobalNotifQuery(db, state.User))
	if len(state.DeletedMsgIDList) > 0 {
		query = query.Where("id NOT IN ?", state.DeletedMsgIDList)
	}
	return query
}
