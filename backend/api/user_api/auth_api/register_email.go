package auth_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_user"
	"myblogx/service/user_service"
	"myblogx/utils/pwd"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RegisterEmailRequest struct {
	Pwd string `json:"pwd" binding:"required"`
}

func (h AuthApi) RegisterEmailView(c *gin.Context) {
	app := h.App
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)
	if app.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	if !app.RuntimeSite.GetRuntimeLogin().EmailLogin {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, "", 0, "站点未启用邮箱注册", nil)
		res.FailWithMsg("站点未启用邮箱注册功能", c)
		return
	}

	cr := middleware.GetBindJson[RegisterEmailRequest](c)

	email := c.GetString("email")
	if email == "" {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, "", 0, "邮箱验证失败：邮箱不存在", nil)
		res.FailWithMsg("邮箱验证失败：邮箱不存在", c)
		return
	}

	// 注册用户
	hashedPassword, err := pwd.GenerateFromPassword(cr.Pwd)
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱注册失败", nil)
		res.FailWithMsg("邮箱注册失败", c)
		return
	}
	username, err := redis_user.NextAutoUsername(redisDeps)
	if err != nil {
		app.Logger.Errorf("邮箱注册生成用户名失败: %v", err)
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱注册失败", nil)
		res.FailWithMsg("邮箱注册失败", c)
		return
	}

	var user models.UserModel
	for range 5 {
		emailValue := email
		user = models.UserModel{
			Username:       username,
			Password:       hashedPassword,
			Nickname:       email,
			Avatar:         "xxx.png",
			RegisterSource: enum.RegisterEmailSourceType,
			Email:          &emailValue,
			Role:           enum.RoleUser,
		}
		var resultRows int64
		err = app.DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "email"}},
				DoNothing: true,
			}).Create(&user)
			if result.Error != nil {
				return result.Error
			}
			resultRows = result.RowsAffected
			if resultRows == 0 {
				return nil
			}
			return user_service.InitUserDefaults(tx, user.ID)
		})
		result := struct {
			Error        error
			RowsAffected int64
		}{Error: err, RowsAffected: resultRows}
		if result.Error == nil {
			if result.RowsAffected == 0 {
				middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱已被使用", nil)
				res.FailWithMsg("邮箱已被使用", c)
				return
			}
			break
		}
		if !errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			err = result.Error
			break
		}

		username, err = redis_user.NextAutoUsername(redisDeps)
		if err != nil {
			app.Logger.Errorf("邮箱注册生成用户名失败: %v", err)
			middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱注册失败", nil)
			res.FailWithMsg("邮箱注册失败", c)
			return
		}
	}
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱注册失败", nil)
		res.FailWithMsg("邮箱注册失败", c)
		app.Logger.Errorf("邮箱注册失败 %v", err)
		return
	}
	if user.ID == 0 {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, email, 0, "邮箱注册失败", nil)
		res.FailWithMsg("邮箱注册失败", c)
		app.Logger.Errorf("邮箱注册失败: 自动用户名重试次数耗尽")
		return
	}

	deps := user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis)
	jwtToken, refreshToken, _, err := user_service.CreateLoginTokens(deps, &user, buildSessionMeta(c))
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "register_fail", enum.EmailLoginType, false, user.Username, user.ID, "邮箱登录失败", nil)
		res.FailWithMsg("邮箱登录失败", c)
		return
	}
	user_service.SetRefreshTokenCookie(c.Writer, refreshToken, deps)
	middleware.EmitLoginEventFromGin(c, "register_success", enum.EmailLoginType, true, user.Username, user.ID, "", map[string]any{
		"email": email,
	})
	middleware.EmitLoginEventFromGin(c, "login_success", enum.EmailLoginType, true, user.Username, user.ID, "", map[string]any{
		"email": email,
	})

	// 返回token
	res.OkWithData(jwtToken, c)
}
