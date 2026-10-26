package domain

// Role 是操作者的角色。取值与 models/enum/role_type.go 一致。
type Role int8

const (
	// RoleAnonymous 未登录。
	RoleAnonymous Role = 0
	// RoleAdmin 管理员。
	RoleAdmin Role = 1
	// RoleUser 普通用户。
	RoleUser Role = 2
	// RoleGuest 访客（已登录但无正式身份）。
	RoleGuest Role = 3
)

// Viewer 是"正在看文章的人"。
//
// 可见性规则的入参只有它——不再依赖 Gin 上下文、JWT claims 或数据库查询。
// 把鉴权结果塞进 Viewer 是 HTTP adapter 的职责。
type Viewer struct {
	// UserID 为 0 表示匿名。
	UserID UserID
	// Role 是操作者角色。
	Role Role
}

// Anonymous 返回匿名访客。
func Anonymous() Viewer { return Viewer{Role: RoleAnonymous} }

// NormalUser 返回普通用户视图。
func NormalUser(id UserID) Viewer { return Viewer{UserID: id, Role: RoleUser} }

// AdminUser 返回管理员视图。
func AdminUser(id UserID) Viewer { return Viewer{UserID: id, Role: RoleAdmin} }

// IsAnonymous 判断是否未登录。
func (v Viewer) IsAnonymous() bool { return v.UserID.IsZero() && v.Role == RoleAnonymous }

// IsAdmin 判断是否管理员。
func (v Viewer) IsAdmin() bool { return v.Role == RoleAdmin }

// IsLoggedIn 判断是否已登录。
func (v Viewer) IsLoggedIn() bool { return !v.UserID.IsZero() }
