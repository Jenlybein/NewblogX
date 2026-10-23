package category

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/redis_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// 查询分类列表
func (h CategoryApi) CategoryListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CategoryListRequest](c)

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

	preloads := []string{"ArticleList"}

	switch cr.Type {
	case 1:
		if err != nil {
			res.FailWithError(err, c)
			return
		}
		cr.UserID = claim.UserID
	case 2: //
	case 3:
		if err != nil || claim.IsAdmin() == false {
			res.FailWithMsg("权限不足", c)
			return
		}
		preloads = append(preloads, "UserModel")
	}

	_list, count, err := common.ListQuery(models.CategoryModel{
		UserID: cr.UserID,
	}, common.Options{
		DB:       h.App.DB,
		PageInfo: cr.PageInfo,
		Likes:    []string{"title"},
		Preloads: preloads,
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	var list = make([]CategoryListResponse, 0)
	for _, item := range _list {
		list = append(list, CategoryListResponse{
			CategoryModel: item,
			ArticleCount:  len(item.ArticleList),
			Nickname:      item.UserModel.Nickname,
			Avatar:        item.UserModel.Avatar,
		})
	}

	res.OkWithList(list, count, c)
}
