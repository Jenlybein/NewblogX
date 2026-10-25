package enum

type RoleType int8

const (
	RoleAdmin     RoleType    = iota + 1 // 管理员
	RoleUser                             // 普通用户
	RoleGuest                            // 访客
	RoleTypeCount = int(iota)            // 角色类型总数
)

func (r RoleType) String() string {
	switch r {
	case RoleAdmin:
		return "管理员"
	case RoleUser:
		return "普通用户"
	case RoleGuest:
		return "访客"
	default:
		return "未知角色"
	}
}
