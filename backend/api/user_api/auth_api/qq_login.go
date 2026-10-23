package auth_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/qq_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_user"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QQLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

func (h AuthApi) QQLoginView(c *gin.Context) {
	app := h.App
	redisDeps := redis_service.NewDeps(h.App.Redis, h.App.Logger)
	if app.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}
	if !app.RuntimeSite.GetRuntimeLogin().QQLogin {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, "", 0, "站点未启用QQ登录", nil)
		res.FailWithMsg("站点未启用qq登录功能", c)
		return
	}

	cr := middleware.GetBindJson[QQLoginRequest](c)

	userInfoResp, err := qq_service.GetUserInfo(app.QQ, cr.Code)
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, "", 0, err.Error(), nil)
		res.FailWithError(err, c)
		return
	}

	var user models.UserModel
	err = app.DB.Take(&user, "open_id = ?", userInfoResp.OpenID).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, userInfoResp.NickName, 0, "qq登录失败 "+err.Error(), map[string]any{
				"open_id": userInfoResp.OpenID,
			})
			res.FailWithMsg("qq登录失败 "+err.Error(), c)
			return
		}

		for range 5 {
			username, usernameErr := redis_user.NextAutoUsername(redisDeps)
			if usernameErr != nil {
				app.Logger.Errorf("QQ 登录生成用户名失败: %v", usernameErr)
				middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, userInfoResp.NickName, 0, "qq登录失败", map[string]any{
					"open_id": userInfoResp.OpenID,
				})
				res.FailWithMsg("qq登录失败", c)
				return
			}

			openID := userInfoResp.OpenID
			user = models.UserModel{
				Username:       username,
				Nickname:       userInfoResp.NickName,
				Avatar:         userInfoResp.Avatar,
				RegisterSource: enum.RegisterQQSourceType,
				OpenID:         &openID,
				Role:           enum.RoleUser,
			}
			var resultRows int64
			err = app.DB.Transaction(func(tx *gorm.DB) error {
				result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "open_id"}},
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
					if err = app.DB.Take(&user, "open_id = ?", userInfoResp.OpenID).Error; err != nil {
						middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, userInfoResp.NickName, 0, "qq登录失败 "+err.Error(), map[string]any{
							"open_id": userInfoResp.OpenID,
						})
						res.FailWithMsg("qq登录失败 "+err.Error(), c)
						return
					}
				}
				break
			}
			if !errors.Is(result.Error, gorm.ErrDuplicatedKey) {
				middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, userInfoResp.NickName, 0, "qq登录失败 "+result.Error.Error(), map[string]any{
					"open_id": userInfoResp.OpenID,
				})
				res.FailWithMsg("qq登录失败 "+result.Error.Error(), c)
				return
			}
		}
		if user.ID == 0 {
			middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, userInfoResp.NickName, 0, "qq登录失败", map[string]any{
				"open_id": userInfoResp.OpenID,
			})
			res.FailWithMsg("qq登录失败", c)
			return
		}
	}

	if !user.CanLogin() {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, user.Username, user.ID, user.Status.String(), map[string]any{
			"open_id": userInfoResp.OpenID,
		})
		res.FailWithMsg(user.Status.String(), c)
		return
	}

	deps := user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis)
	token, refreshToken, _, err := user_service.CreateLoginTokens(deps, &user, buildSessionMeta(c))
	if err != nil {
		middleware.EmitLoginEventFromGin(c, "login_fail", enum.QQLoginType, false, user.Username, user.ID, "qq登录失败 "+err.Error(), map[string]any{
			"open_id": userInfoResp.OpenID,
		})
		res.FailWithMsg("qq登录失败 "+err.Error(), c)
		return
	}
	user_service.SetRefreshTokenCookie(c.Writer, refreshToken, deps)
	middleware.EmitLoginEventFromGin(c, "login_success", enum.QQLoginType, true, user.Username, user.ID, "", map[string]any{
		"open_id": userInfoResp.OpenID,
	})

	res.OkWithData(token, c)
}
