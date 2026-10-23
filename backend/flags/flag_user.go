package flags

import (
	"fmt"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/utils/pwd"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"gorm.io/gorm"
)

type FlagUser struct{}

type UserCreateOptions struct {
	Role           string
	Username       string
	Password       string
	Nickname       string
	Email          string
	NonInteractive bool
}

func (u *FlagUser) Create(db *gorm.DB, logger *logrus.Logger, opts UserCreateOptions) {
	if shouldUseNonInteractive(opts) {
		u.createNonInteractive(db, logger, opts)
		return
	}

	var role enum.RoleType
	fmt.Println("请输入数字选择用户角色: ")
	for r := 1; r <= enum.RoleTypeCount; r++ {
		fmt.Printf("%d. %s\n", r, enum.RoleType(r).String())
	}
	if _, err := fmt.Scanln(&role); err != nil {
		fmt.Printf("输入类型错误 %s\n", err.Error())
		return
	}
	if int(role) < int(enum.RoleAdmin) || int(role) > enum.RoleTypeCount {
		fmt.Printf("输入角色类型错误 %d\n", role)
		return
	}

	var username string
	fmt.Println("请输入用户名: ")
	fmt.Scanln(&username)

	var model models.UserModel
	if db.Take(&model, "username = ?", username).Error == nil {
		fmt.Printf("用户名 %s 已存在\n", username)
		return
	}

	fmt.Println("请输入密码: ")
	password, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Printf("读取密码错误 %s\n", err.Error())
		return
	}
	fmt.Println("请确认密码: ")
	rePassword, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Printf("读取密码错误 %s\n", err.Error())
		return
	}
	if string(password) != string(rePassword) {
		fmt.Println("两次密码输入不一致")
		return
	}

	// 密码加密
	hashedPassword, err := pwd.GenerateFromPassword(string(password))
	if err != nil {
		fmt.Printf("密码加密错误 %s\n", err.Error())
		return
	}

	// 创建用户
	if err := db.Create(&models.UserModel{
		Username:       username,
		Nickname:       "命令用户",
		Password:       hashedPassword,
		Role:           role,
		RegisterSource: enum.RegisterTerminalSourceType,
	}).Error; err != nil {
		fmt.Printf("创建用户错误 %s\n", err.Error())
		return
	}
	msg := fmt.Sprintf("用户 %s 创建成功\n", username)
	if logger != nil {
		logger.Info(msg)
	}
}

func shouldUseNonInteractive(opts UserCreateOptions) bool {
	if opts.NonInteractive {
		return true
	}
	return strings.TrimSpace(opts.Role) != "" ||
		strings.TrimSpace(opts.Username) != "" ||
		strings.TrimSpace(opts.Password) != "" ||
		strings.TrimSpace(opts.Nickname) != "" ||
		strings.TrimSpace(opts.Email) != ""
}

func (u *FlagUser) createNonInteractive(db *gorm.DB, logger *logrus.Logger, opts UserCreateOptions) {
	role, err := parseRoleInput(opts.Role)
	if err != nil {
		fmt.Printf("角色参数错误: %s\n", err.Error())
		return
	}

	username := strings.TrimSpace(opts.Username)
	if username == "" {
		fmt.Println("参数错误: user-username 不能为空")
		return
	}

	password := strings.TrimSpace(opts.Password)
	if password == "" {
		fmt.Println("参数错误: user-password 不能为空")
		return
	}

	nickname := strings.TrimSpace(opts.Nickname)
	if nickname == "" {
		nickname = "命令用户"
	}

	var model models.UserModel
	if db.Take(&model, "username = ?", username).Error == nil {
		fmt.Printf("用户名 %s 已存在\n", username)
		return
	}

	hashedPassword, err := pwd.GenerateFromPassword(password)
	if err != nil {
		fmt.Printf("密码加密错误 %s\n", err.Error())
		return
	}

	user := models.UserModel{
		Username:       username,
		Nickname:       nickname,
		Password:       hashedPassword,
		Role:           role,
		RegisterSource: enum.RegisterTerminalSourceType,
	}

	email := strings.TrimSpace(opts.Email)
	if email != "" {
		user.Email = &email
	}

	if err = db.Create(&user).Error; err != nil {
		fmt.Printf("创建用户错误 %s\n", err.Error())
		return
	}

	msg := fmt.Sprintf("用户 %s 创建成功\n", username)
	if logger != nil {
		logger.Info(msg)
	}
	fmt.Print(msg)
}

func parseRoleInput(raw string) (enum.RoleType, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("user-role 不能为空，可用值: admin|user|guest 或 1|2|3")
	}

	if num, err := strconv.Atoi(trimmed); err == nil {
		role := enum.RoleType(num)
		if int(role) < int(enum.RoleAdmin) || int(role) > enum.RoleTypeCount {
			return 0, fmt.Errorf("不支持的角色编号 %d", num)
		}
		return role, nil
	}

	switch strings.ToLower(trimmed) {
	case "admin", "管理员":
		return enum.RoleAdmin, nil
	case "user", "普通用户":
		return enum.RoleUser, nil
	case "guest", "访客":
		return enum.RoleGuest, nil
	default:
		return 0, fmt.Errorf("不支持的角色 %q", raw)
	}
}
