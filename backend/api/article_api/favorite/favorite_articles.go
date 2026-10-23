package favorite

import (
	"errors"
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

var favoriteArticleOrderMap = map[string]string{
	"created_at desc": "user_article_favor_models.created_at desc",
	"created_at asc":  "user_article_favor_models.created_at asc",
}

// FavoriteArticlesView 查询某个收藏夹内的文章列表。
// 1. 收藏夹所有者和管理员可直接查看
// 2. 其他人只有在收藏夹公开时才能查看
// 3. 列表按收藏时间排序，支持按文章标题关键字筛选
func (h FavoriteApi) FavoriteArticlesView(c *gin.Context) {
	cr := middleware.GetBindQuery[FavoriteArticlesRequest](c)
	var claims *jwts.MyClaims
	token := jwts.GetTokenByGin(c)
	if token != "" {
		authenticator := user_service.NewAuthenticator(
			h.App.DB,
			h.App.Logger,
			h.App.JWT,
			redis_service.Deps{Client: h.App.Redis, Logger: h.App.Logger},
		)
		if authResult, err := authenticator.AuthenticateAccessToken(token); err == nil {
			claims = authResult.Claims
		}
	}

	favoriteModel, err := getAccessibleFavorite(h.App, c, cr.FavoriteID, claims)
	if err != nil {
		return
	}

	queryService := favorite_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))
	list, hasMore, err := queryService.ListFavoriteArticles(favorite_service.FavoriteArticlesQuery{
		PageInfo:   cr.PageInfo,
		FavoriteID: favoriteModel.ID,
	}, favoriteArticleOrderMap)
	if err != nil {
		res.FailWithMsg("查询收藏夹文章失败", c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}

func getAccessibleFavorite(app Deps, c *gin.Context, favoriteID ctype.ID, claims *jwts.MyClaims) (*models.FavoriteModel, error) {
	var favoriteModel models.FavoriteModel
	if err := app.DB.Take(&favoriteModel, "id = ?", favoriteID).Error; err != nil {
		res.FailWithMsg("收藏夹不存在", c)
		return nil, err
	}

	if claims != nil {
		if claims.IsAdmin() || claims.UserID == favoriteModel.UserID {
			return &favoriteModel, nil
		}
	}

	var userConf models.UserConfModel
	if err := app.DB.Take(&userConf, "user_id = ?", favoriteModel.UserID).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		return nil, err
	}
	if !userConf.FavoritesVisibility {
		res.FailWithMsg("收藏夹不公开", c)
		return nil, errors.New("favorite not visible")
	}
	return &favoriteModel, nil
}
