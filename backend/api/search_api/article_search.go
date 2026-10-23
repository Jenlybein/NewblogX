package search_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/search_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"strings"

	"github.com/gin-gonic/gin"
)

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
