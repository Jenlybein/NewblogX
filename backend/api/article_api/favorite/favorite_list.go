package favorite

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/platform/cachex"
	"myblogx/service/favorite_service"
	"myblogx/service/redis_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// 查询收藏夹列表
func (h FavoriteApi) FavoriteListView(c *gin.Context) {
	cr := middleware.GetBindQuery[FavoriteListRequest](c)

	var claim *jwts.MyClaims
	var err error
	token := jwts.GetTokenByGin(c)
	if token == "" {
		err = user_service.ErrAuthRequired
	} else {
		authenticator := user_service.NewAuthenticator(
			h.App.DB,
			h.App.Logger,
			h.App.JWT,
			redis_service.Deps{Client: h.App.Redis, Logger: h.App.Logger},
		)
		if authResult, authErr := authenticator.AuthenticateAccessToken(token); authErr == nil {
			claim = authResult.Claims
		} else {
			err = authErr
		}
	}

	switch cr.Type {
	case 1:
		if err != nil {
			res.FailWithError(err, c)
			return
		}
		cr.UserID = claim.UserID
	case 2:
		if cr.UserID == 0 {
			res.FailWithMsg("用户 id 不能为空", c)
			return
		}
		// 查询目标用户隐私设置，判断是否公开收藏夹
		var userConf models.UserConfModel
		if err := h.App.DB.Take(&userConf, "user_id = ?", cr.UserID).Error; err != nil {
			res.FailWithMsg("用户不存在", c)
			return
		}
		if !userConf.FavoritesVisibility {
			res.FailWithMsg("收藏夹不公开", c)
			return
		}
	case 3:
		if err != nil || claim.IsAdmin() == false {
			res.FailWithMsg("权限不足", c)
			return
		}
	}

	viewerUserID := ctype.ID(0)
	if claim != nil {
		viewerUserID = claim.UserID
	}
	queryService := favorite_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))
	list, count, err := queryService.ListFavorites(favorite_service.FavoriteListQuery{
		PageInfo:  cr.PageInfo,
		UserID:    cr.UserID,
		ViewerID:  viewerUserID,
		ArticleID: cr.ArticleID,
		Type:      cr.Type,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithList(list, count, c)
}
