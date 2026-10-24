package category

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/internal/platform/redis"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// 查询分类列表
// CategoryListView 分页查询分类列表。
// @Summary 分类列表
// @Tags article
// @Produce json
// @Param page query int false "页码"
// @Param limit query int false "每页条数"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param user_id query int false "目标用户 ID"
// @Param type query int true "查询类型(1:自己 2:别人 3:管理员)"
// @Success 200 {object} object{code=int,data=object{list=[]category.CategoryListResponse,count=int},msg=string}
// @Router /articles/category [get]
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
