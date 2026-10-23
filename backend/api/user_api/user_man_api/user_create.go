package user_man_api

import (
	"errors"
	"strings"

	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/user_service"
	"myblogx/utils/pwd"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminUserCreateRequest struct {
	Username string `json:"username" binding:"required,min=2,max=32"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"omitempty,min=2,max=32"`
	Email    string `json:"email" binding:"omitempty,email,max=256"`
}

type AdminUserCreateResponse struct {
	ID             ctype.ID                `json:"id"`
	Username       string                  `json:"username"`
	Nickname       string                  `json:"nickname"`
	Email          *string                 `json:"email"`
	Role           enum.RoleType           `json:"role"`
	Status         enum.UserStatus         `json:"status"`
	RegisterSource enum.RegisterSourceType `json:"register_source"`
}

func (h *UserManApi) AdminUserCreateView(c *gin.Context) {
	cr := middleware.GetBindJson[AdminUserCreateRequest](c)

	username := strings.TrimSpace(cr.Username)
	nickname := strings.TrimSpace(cr.Nickname)
	if nickname == "" {
		nickname = username
	}
	emailRaw := strings.TrimSpace(cr.Email)

	if err := h.ensureAdminUserCreateUnique(username, emailRaw); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	hashedPassword, err := pwd.GenerateFromPassword(cr.Password)
	if err != nil {
		res.FailWithMsg("创建用户失败", c)
		return
	}

	user := models.UserModel{
		Username:       username,
		Nickname:       nickname,
		Password:       hashedPassword,
		Role:           enum.RoleUser,
		Status:         enum.UserStatusActive,
		RegisterSource: enum.RegisterAdminSourceType,
	}
	if emailRaw != "" {
		user.Email = &emailRaw
	}

	if err = h.App.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return user_service.InitUserDefaults(tx, user.ID)
	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			res.FailWithMsg("用户名或邮箱已存在", c)
			return
		}
		res.FailWithMsg("创建用户失败", c)
		return
	}

	res.OkWithData(AdminUserCreateResponse{
		ID:             user.ID,
		Username:       user.Username,
		Nickname:       user.Nickname,
		Email:          user.Email,
		Role:           user.Role,
		Status:         user.Status,
		RegisterSource: user.RegisterSource,
	}, c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "admin_user_create",
		TargetType: "user",
		TargetID:   user.ID.String(),
		Success:    true,
		Message:    "管理员创建用户成功",
		RequestBody: map[string]any{
			"username": username,
			"nickname": nickname,
			"email":    user.Email,
			"role":     user.Role,
		},
		ResponseBody: map[string]any{
			"id":              user.ID,
			"username":        user.Username,
			"nickname":        user.Nickname,
			"email":           user.Email,
			"role":            user.Role,
			"status":          user.Status,
			"register_source": user.RegisterSource,
		},
		UseRawRequestHead: true,
	})
}

func (h *UserManApi) ensureAdminUserCreateUnique(username string, email string) error {
	var count int64
	if err := h.App.DB.Model(&models.UserModel{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return errors.New("查询用户名失败")
	}
	if count > 0 {
		return errors.New("用户名已存在")
	}
	if email == "" {
		return nil
	}
	if err := h.App.DB.Model(&models.UserModel{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return errors.New("查询邮箱失败")
	}
	if count > 0 {
		return errors.New("邮箱已存在")
	}
	return nil
}
