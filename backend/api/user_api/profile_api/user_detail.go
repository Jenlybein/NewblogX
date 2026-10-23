package profile_api

import (
	"myblogx/common/res"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/image_service"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserLikeTagItem struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

type UserDetailResponse struct {
	ID                       ctype.ID                `gorm:"primaryKey" json:"id"`
	CreatedAt                time.Time               `json:"created_at"`
	Username                 string                  `gorm:"size:32" json:"username"`
	Nickname                 string                  `gorm:"size:32" json:"nickname"`
	AvatarImageID            *ctype.ID               `json:"avatar_image_id,omitempty"`
	Avatar                   string                  `gorm:"size:256" json:"avatar"`
	Abstract                 string                  `gorm:"size:256" json:"abstract"`
	Email                    *string                 `json:"email"`
	HasPassword              bool                    `json:"has_password"`
	RegisterSource           enum.RegisterSourceType `json:"register_source"`
	CodeAge                  int                     `json:"code_age"`
	LikeTagIDs               []ctype.ID              `json:"like_tag_ids"`
	LikeTagItems             []UserLikeTagItem       `json:"like_tag_items"`
	UpdatedUsernameDate      *time.Time              `json:"updated_username_date"`
	FavoritesVisibility      bool                    `json:"favorites_visibility"`
	FollowVisibility         bool                    `json:"followers_visibility"`
	FansVisibility           bool                    `json:"fans_visibility"`
	HomeStyleID              ctype.ID                `json:"home_style_id"`
	DiggNoticeEnabled        bool                    `json:"digg_notice_enabled"`
	CommentNoticeEnabled     bool                    `json:"comment_notice_enabled"`
	FavorNoticeEnabled       bool                    `json:"favor_notice_enabled"`
	PrivateChatNoticeEnabled bool                    `json:"private_chat_notice_enabled"`
	StrangerChatEnabled      bool                    `json:"stranger_msg_enabled"`
}

func (h ProfileApi) UserDetailView(c *gin.Context) {
	app := h.App
	claims := jwts.MustGetClaimsByGin(c)

	var user models.UserModel
	if err := app.DB.Preload("UserConfModel").Take(&user, claims.UserID).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		c.Abort()
		return
	}

	var likeTagIDs []ctype.ID
	if user.UserConfModel != nil {
		likeTagIDs = user.UserConfModel.LikeTags
	}
	likeTagItems, err := loadUserLikeTagItems(app.DB, likeTagIDs)
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	var data = UserDetailResponse{
		ID:             user.ID,
		CreatedAt:      user.CreatedAt,
		Username:       user.Username,
		Nickname:       user.Nickname,
		Avatar:         user.Avatar,
		Abstract:       user.Abstract,
		Email:          user.Email,
		HasPassword:    user.Password != "",
		RegisterSource: user.RegisterSource,
		CodeAge:        user.CodeAge(),
		LikeTagIDs:     likeTagIDs,
		LikeTagItems:   likeTagItems,
	}
	if user.UserConfModel != nil {
		data.UpdatedUsernameDate = user.UserConfModel.UpdatedUsernameDate
		data.FavoritesVisibility = user.UserConfModel.FavoritesVisibility
		data.FollowVisibility = user.UserConfModel.FollowVisibility
		data.FansVisibility = user.UserConfModel.FansVisibility
		data.HomeStyleID = user.UserConfModel.HomeStyleID
		data.DiggNoticeEnabled = user.UserConfModel.DiggNoticeEnabled
		data.CommentNoticeEnabled = user.UserConfModel.CommentNoticeEnabled
		data.FavorNoticeEnabled = user.UserConfModel.FavorNoticeEnabled
		data.PrivateChatNoticeEnabled = user.UserConfModel.PrivateChatNoticeEnabled
		data.StrangerChatEnabled = user.UserConfModel.StrangerChatEnabled
	}
	data.AvatarImageID, _ = image_service.FindImageIDByURL(app.DB, user.Avatar)

	res.OkWithData(data, c)
}

func loadUserLikeTagItems(db *gorm.DB, likeTagIDs []ctype.ID) ([]UserLikeTagItem, error) {
	normalized := normalizeIDs(likeTagIDs)
	if len(normalized) == 0 {
		return []UserLikeTagItem{}, nil
	}

	var tagList []models.TagModel
	if err := db.Select("id", "title").Find(&tagList, "id IN ?", normalized).Error; err != nil {
		return nil, err
	}

	titleMap := make(map[ctype.ID]string, len(tagList))
	for _, item := range tagList {
		titleMap[item.ID] = item.Title
	}

	resp := make([]UserLikeTagItem, 0, len(normalized))
	for _, id := range normalized {
		title, ok := titleMap[id]
		if !ok {
			continue
		}
		resp = append(resp, UserLikeTagItem{
			ID:    id,
			Title: title,
		})
	}
	return resp, nil
}
