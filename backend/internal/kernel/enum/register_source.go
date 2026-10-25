package enum

type RegisterSourceType int8

const (
	RegisterEmailSourceType    RegisterSourceType = 1 // 邮箱注册
	RegisterQQSourceType       RegisterSourceType = 2 // qq 登录
	RegisterTerminalSourceType RegisterSourceType = 3 // 终端注册
	RegisterAdminSourceType    RegisterSourceType = 4 // 管理员创建
)
