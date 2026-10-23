package profile_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/follow_service"
	"myblogx/service/image_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

type UserBaseInfoResponse struct {
	ID                  ctype.ID  `gorm:"primaryKey" json:"id"`
	CodeAge             int       `json:"code_age"`
	AvatarImageID       *ctype.ID `json:"avatar_image_id,omitempty"`
	Avatar              string    `gorm:"size:256" json:"avatar"`
	Nickname            string    `gorm:"size:32" json:"nickname"`
	Abstract            string    `gorm:"size:256" json:"abstract"`
	ViewCount           int       `json:"view_count"`
	FansCount           int       `json:"fans_count"`
	FollowCount         int       `json:"follow_count"`
	ArticleVisitedCount int       `json:"article_visited_count"`
	ArticleCount        int       `json:"article_count"`
	FavoritesVisibility bool      `json:"favorites_visibility"`
	FollowVisibility    bool      `json:"followers_visibility"`
	FansVisibility      bool      `json:"fans_visibility"`
	HomeStyleID         ctype.ID  `json:"home_style_id"`
	Relation            int8      `json:"relation"`
	Place               string    `json:"place"`
}

func (h ProfileApi) UserBaseInfoView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindQuery[models.IDRequest](c)

	var user models.UserModel
	if err := app.DB.Preload("UserConfModel").Preload("UserStatModel").Take(&user, cr.ID).Error; err != nil {
		res.FailWithError(err, c)
		return
	}

	viewCountDelta := 0
	if claims := jwts.GetClaimsByGin(c); claims != nil && claims.UserID != 0 && claims.UserID != user.ID {
		counted, err := user_service.StatRecordUserHomeView(user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis), user.ID, claims.UserID)
		if err != nil {
			res.FailWithError(err, c)
			return
		}
		if counted {
			viewCountDelta = 1
		}
	}

	var stat models.UserStatModel
	if user.UserStatModel != nil {
		stat = *user.UserStatModel
	}
	var conf models.UserConfModel
	if user.UserConfModel != nil {
		conf = *user.UserConfModel
	}
	relation := int8(0)
	if claims := jwts.GetClaimsByGin(c); claims != nil {
		relation = int8(follow_service.CalUserRelationship(app.DB, claims.UserID, user.ID))
	}

	data := UserBaseInfoResponse{
		ID:                  user.ID,
		CodeAge:             user.CodeAge(),
		Avatar:              user.Avatar,
		Nickname:            user.Nickname,
		Abstract:            user.Abstract,
		ViewCount:           stat.ViewCount + viewCountDelta,
		FansCount:           stat.FansCount,
		FollowCount:         stat.FollowCount,
		ArticleVisitedCount: stat.ArticleVisitedCount,
		ArticleCount:        stat.ArticleCount,
		FavoritesVisibility: conf.FavoritesVisibility,
		FollowVisibility:    conf.FollowVisibility,
		FansVisibility:      conf.FansVisibility,
		HomeStyleID:         conf.HomeStyleID,
		Relation:            relation,
		Place:               user.Addr,
	}
	data.AvatarImageID, _ = image_service.FindImageIDByURL(app.DB, user.Avatar)

	res.OkWithData(data, c)
}
