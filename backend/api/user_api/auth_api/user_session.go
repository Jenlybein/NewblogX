package auth_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service/redis_jwt"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"
	"time"

	"github.com/gin-gonic/gin"
)

type UserSessionListRequest struct {
	common.PageInfo
}

type UserSessionItem struct {
	ID         ctype.ID   `json:"id"`
	IP         string     `json:"ip"`
	Addr       string     `json:"addr"`
	UA         string     `json:"ua"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	IsCurrent  bool       `json:"is_current"`
}

// UserSessionListView 返回当前用户仍然有效的登录会话列表。
func (h AuthApi) UserSessionListView(c *gin.Context) {
	cr := middleware.GetBindQuery[UserSessionListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	list, count, err := common.ListQuery(models.UserSessionModel{
		UserID: claims.UserID,
	}, common.Options{
		DB: h.App.DB,
		PageInfo: common.PageInfo{
			Page:  cr.Page,
			Limit: cr.Limit,
		},
		Select:       []string{"id", "ip", "addr", "ua", "created_at", "last_seen_at", "expires_at"},
		Where:        h.App.DB.Where("revoked_at IS NULL AND expires_at > ?", time.Now()),
		DefaultOrder: "created_at desc",
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	resp := make([]UserSessionItem, 0, len(list))
	for _, item := range list {
		resp = append(resp, UserSessionItem{
			ID:         item.ID,
			IP:         item.IP,
			Addr:       item.Addr,
			UA:         item.UA,
			CreatedAt:  item.CreatedAt,
			LastSeenAt: item.LastSeenAt,
			ExpiresAt:  item.ExpiresAt,
			IsCurrent:  item.ID == claims.SessionID,
		})
	}

	res.OkWithList(resp, count, c)
}

// UserSessionDeleteView 吊销当前用户指定的登录会话。
func (h AuthApi) UserSessionDeleteView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	var session models.UserSessionModel
	if err := h.App.DB.Select("id").
		Take(&session, "id = ? AND user_id = ? AND revoked_at IS NULL", cr.ID, claims.UserID).Error; err != nil {
		res.FailWithMsg("会话不存在或已失效", c)
		return
	}

	deps := user_service.NewDepsWithRedis(h.App.JWT, h.App.System.Env, h.App.DB, h.App.Logger, h.App.Redis)
	if err := user_service.RevokeSessionByID(deps, claims.UserID, cr.ID); err != nil {
		res.FailWithError(err, c)
		return
	}

	if cr.ID == claims.SessionID {
		if token := jwts.GetTokenByGin(c); token != "" {
			redis_jwt.SetTokenBlack(deps.Redis, deps.JWT, token, redis_jwt.UserBlackType)
		}
		user_service.ClearRefreshTokenCookie(c.Writer, deps)
		res.OkWithMsg("当前设备已下线", c)
		return
	}

	res.OkWithMsg("设备已下线", c)
}
