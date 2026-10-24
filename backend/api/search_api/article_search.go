package search_api

import (
	"myblogx/common/res"
	"myblogx/internal/platform/redis"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/service/search_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"strings"

	"github.com/gin-gonic/gin"
)

// ArticleSearchView 按关键词、标签、分类等条件搜索文章。
// @Summary 文章搜索
// @Tags search
// @Produce json
// @Param type query int false "搜索类型：1 公共文章 2 猜你喜欢 3 作者文章 4 自己文章 5 管理员搜索"
// @Param sort query int false "排序：1 默认 2 最新发布 3 最多回复 4 最多点赞 5 最多收藏 6 最多浏览"
// @Param page_mode query string false "分页模式"
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Param key query string false "搜索关键词"
// @Param category_id query int false "分类 ID"
// @Param author_id query int false "作者 ID"
// @Param status query int false "文章状态"
// @Param tag_ids query string false "标签 ID 列表，支持逗号分隔"
// @Success 200 {object} object{code=int,data=search_service.ArticleSearchResponse,msg=string}
// @Router /search/articles [get]
func (h SearchApi) ArticleSearchView(c *gin.Context) {
	cr := middleware.GetBindQuery[search_service.ArticleSearchRequest](c)
	if len(cr.TagIDs) == 0 {
		cr.TagIDs = parseTagIDsFromGin(c)
	}

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

	app := h.App
	var likeTagIDs []ctype.ID
	if claims != nil && cr.NormalizeType() == 2 {
		if tags, err := search_service.LoadUserLikeTagIDs(app.DB, claims.UserID); err == nil {
			likeTagIDs = tags
		} else {
			app.Logger.Warnf("加载用户偏好标签失败，降级为无偏好搜索: user_id=%d err=%v", claims.UserID, err)
		}
	}

	result, err := search_service.SearchArticles(cr, claims, likeTagIDs, redis_service.NewDeps(h.App.Redis, h.App.Logger), app.ESClient, app.ES.Index)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(result, c)
}

func parseTagIDsFromGin(c *gin.Context) []ctype.ID {
	rawList := c.QueryArray("tag_ids")
	if len(rawList) == 0 {
		if joined := strings.TrimSpace(c.Query("tag_ids")); joined != "" {
			rawList = strings.Split(joined, ",")
		}
	}

	result := make([]ctype.ID, 0, len(rawList))
	seen := make(map[ctype.ID]struct{}, len(rawList))
	for _, raw := range rawList {
		var id ctype.ID
		if err := id.UnmarshalText([]byte(strings.TrimSpace(raw))); err != nil || id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
