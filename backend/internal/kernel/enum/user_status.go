package enum

type UserStatus int8

const (
	UserStatusActive   UserStatus = 1 // 正常
	UserStatusDisabled UserStatus = 2 // 禁用
	UserStatusBanned   UserStatus = 3 // 封禁
)

func (s UserStatus) String() string {
	switch s {
	case UserStatusActive:
		return "正常"
	case UserStatusDisabled:
		return "账号已被禁用"
	case UserStatusBanned:
		return "账号已被封禁"
	default:
		return "未知"
	}
}

func (s UserStatus) CanLogin() bool {
	return s == UserStatusActive
}
