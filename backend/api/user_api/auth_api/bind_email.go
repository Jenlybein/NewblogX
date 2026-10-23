package auth_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h AuthApi) BindEmailView(c *gin.Context) {
	app := h.App
	email := c.GetString("email")
	if email == "" {
		res.FailWithMsg("邮箱验证失败：邮箱不存在", c)
		return
	}

	claims := jwts.MustGetClaimsByGin(c)

	var user models.UserModel
	if err := app.DB.Take(&user, claims.UserID).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}

	if err := app.DB.Model(&user).Update("email", email).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			res.FailWithMsg("邮箱已被使用", c)
			return
		}
		res.FailWithError(err, c)
		return
	}

	res.OkWithMsg("邮箱绑定成功", c)
}
