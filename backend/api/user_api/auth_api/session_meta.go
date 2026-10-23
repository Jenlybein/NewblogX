package auth_api

import (
	"myblogx/service/user_service"
	"myblogx/utils/requestmeta"

	"github.com/gin-gonic/gin"
)

func buildSessionMeta(c *gin.Context) user_service.SessionMeta {
	meta := requestmeta.BuildSessionMeta(c)
	return user_service.SessionMeta{
		IP:   meta.IP,
		Addr: meta.Addr,
		UA:   meta.UA,
	}
}
