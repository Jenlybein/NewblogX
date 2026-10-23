package global_notif_enum

type Type int8

// 用户可见规则：
// 1-仅发布且未过期时已注册的用户，即老用户（默认）
// 2-仅发布且未过期时才注册的用户，即新用户
// 3-所有用户

const (
	UserVisibleRegisteredUsers Type = iota + 1
	UserVisibleNewUsers
	UserVisibleAllUsers
)
