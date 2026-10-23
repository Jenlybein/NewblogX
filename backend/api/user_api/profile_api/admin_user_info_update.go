package profile_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/image_service"
	"myblogx/service/read_service"
	"myblogx/service/user_service"
	"myblogx/utils/maps"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AdminUserInfoUpdateRequest struct {
	UserID        ctype.ID         `json:"user_id" binding:"required"`
	Username      *string          `json:"username"`
	Nickname      *string          `json:"nickname"`
	AvatarImageID *ctype.ID        `json:"avatar_image_id"`
	Abstract      *string          `json:"abstract"`
	Role          *enum.RoleType   `json:"role"`
	Status        *enum.UserStatus `json:"status"`
}

func (h ProfileApi) AdminUserInfoUpdateView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[AdminUserInfoUpdateRequest](c)

	userMap, err := maps.FieldsStructToMap(&cr, &models.UserModel{})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	var userModel models.UserModel
	if err = app.DB.Take(&userModel, cr.UserID).Error; err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}
	if cr.AvatarImageID != nil {
		avatarURL, err := image_service.ResolveImageURLByID(app.DB, *cr.AvatarImageID)
		if err != nil {
			res.FailWithMsg(err.Error(), c)
			return
		}
		userMap["avatar"] = avatarURL
	}

	if err = app.DB.Model(&userModel).Updates(userMap).Error; err != nil {
		res.FailWithMsg("用户信息更新失败", c)
		return
	}
	if cr.Nickname != nil || cr.AvatarImageID != nil || cr.Abstract != nil {
		if err = read_service.SyncUserDisplaySnapshots(app.DB, cr.UserID); err != nil {
			app.Logger.Errorf("同步用户展示快照失败: 用户ID=%d 错误=%v", cr.UserID, err)
		}
	}

	if (cr.Role != nil && *cr.Role != userModel.Role) || (cr.Status != nil && *cr.Status != userModel.Status) {
		if err = user_service.InvalidateUserAuthState(user_service.NewDepsWithRedis(app.JWT, app.System.Env, app.DB, app.Logger, app.Redis), &userModel); err != nil {
			res.FailWithMsg("用户信息更新成功，但会话失效处理失败", c)
			return
		}
	}
	res.OkWithMsg("用户信息更新成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "admin_user_update",
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(cr.UserID), 10),
		Success:    true,
		Message:    "管理员更新用户信息成功",
		RequestBody: map[string]any{
			"user_id":         cr.UserID,
			"username":        cr.Username,
			"nickname":        cr.Nickname,
			"avatar_image_id": cr.AvatarImageID,
			"abstract":        cr.Abstract,
			"role":            cr.Role,
			"status":          cr.Status,
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}
