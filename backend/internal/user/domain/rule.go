package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"myblogx/internal/apperr"
)

// 资料字段的长度上限，与数据库列宽保持一致。
const (
	// MaxUsernameLen 用户名最大长度（字符数，不是字节数）。
	MaxUsernameLen = 32
	// MaxNicknameLen 昵称最大长度。
	MaxNicknameLen = 32
	// MaxAbstractLen 个人简介最大长度。
	MaxAbstractLen = 256
)

// UsernameCooldown 是用户名两次修改之间的最小间隔。
//
// 30 天不是随手定的：用户名出现在文章链接与 @ 提及里，频繁改动会让外部引用失效。
const UsernameCooldown = 30 * 24 * time.Hour

// Viewer 是发起操作的人。
//
// 单独一个类型而不是传 User：多数场景只有 ID 和角色，为判权限去查整行是浪费。
type Viewer struct {
	ID   UserID
	Role Role
}

// Anonymous 返回未登录的操作者。
func Anonymous() Viewer { return Viewer{} }

// IsZero 判断是否未登录。
func (v Viewer) IsZero() bool { return v.ID.IsZero() }

// IsAdmin 判断是否管理员。
func (v Viewer) IsAdmin() bool { return v.Role.IsAdmin() }

// Is 判断操作者是否就是目标用户本人。
func (v Viewer) Is(target UserID) bool { return !v.ID.IsZero() && v.ID == target }

// CanViewProfile 判断能否查看该用户主页。
//
// 规则：只要用户存在就能看。禁用/封禁账号的主页仍然可访问——否则一个用户被误封，
// 他所有历史文章的署名都会变成"用户不存在"，影响的是读者而不是被封的人。
func CanViewProfile(target User) error {
	if target.IsZero() {
		return apperr.New(apperr.CodeUserNotFound)
	}
	return nil
}

// CanUpdateProfile 判断能否修改资料：本人或管理员。
func CanUpdateProfile(target User, viewer Viewer) error {
	if target.IsZero() {
		return apperr.New(apperr.CodeUserNotFound)
	}
	if viewer.IsZero() {
		return apperr.New(apperr.CodeUserForbidden)
	}
	if viewer.Is(target.ID) || viewer.IsAdmin() {
		return nil
	}
	return apperr.New(apperr.CodeUserForbidden)
}

// CanManageUsers 判断能否进入管理端用户列表/创建用户。
func CanManageUsers(viewer Viewer) error {
	if viewer.IsZero() || !viewer.IsAdmin() {
		return apperr.New(apperr.CodeUserForbidden)
	}
	return nil
}

// CanChangeStatus 判断能否修改目标账号的状态。
//
// 两条规则缺一不可：
//  1. 只有管理员能改（普通用户不能封别人，也不能自助解封）；
//  2. **不能改自己**——管理员把自己封了之后没有任何人可以解封，
//     这不是理论风险，是运维事故的常见来源。
func CanChangeStatus(target User, viewer Viewer) error {
	if target.IsZero() {
		return apperr.New(apperr.CodeUserNotFound)
	}
	if viewer.IsZero() || !viewer.IsAdmin() {
		return apperr.New(apperr.CodeUserForbidden)
	}
	if viewer.Is(target.ID) {
		return apperr.New(apperr.CodeUserStatusSelfLocked)
	}
	return nil
}

// ValidateStatus 校验目标状态是否合法。
func ValidateStatus(s Status) error {
	switch s {
	case StatusActive, StatusDisabled, StatusBanned:
		return nil
	default:
		return apperr.New(apperr.CodeUserStatusInvalid)
	}
}

// CanLogin 判断账号能否登录，并把状态翻译成具体原因。
//
// 区分"禁用"和"封禁"不只是文案差异：前者可由管理员一键恢复，后者要走申诉流程，
// 前端据此展示不同的引导。
func CanLogin(u User) error {
	switch u.Status {
	case StatusActive:
		return nil
	case StatusDisabled:
		return apperr.New(apperr.CodeUserAccountDisabled)
	case StatusBanned:
		return apperr.New(apperr.CodeUserAccountBanned)
	default:
		return apperr.New(apperr.CodeUserStatusInvalid)
	}
}

// ValidateUsername 校验用户名格式。
func ValidateUsername(s string) error {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > MaxUsernameLen {
		return apperr.New(apperr.CodeUserUsernameInvalid)
	}
	// 只允许字母、数字、下划线、连字符：用户名会出现在 URL 与 @ 提及里，
	// 放空格或斜杠进去会让链接无法解析。
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
			continue
		default:
			return apperr.New(apperr.CodeUserUsernameInvalid)
		}
	}
	return nil
}

// ValidateNickname 校验昵称。
func ValidateNickname(s string) error {
	s = normalizeNickname(s)
	if s == "" {
		return apperr.New(apperr.CodeUserNicknameRequired)
	}
	if utf8.RuneCountInString(s) > MaxNicknameLen {
		return apperr.New(apperr.CodeUserNicknameTooLong)
	}
	return nil
}

// ValidateAbstract 校验个人简介。
func ValidateAbstract(s string) error {
	if utf8.RuneCountInString(s) > MaxAbstractLen {
		return apperr.New(apperr.CodeUserAbstractTooLong)
	}
	return nil
}

// ProfileUpdate 是一次资料更新请求。
//
// 用指针区分"没传这个字段"和"传了空值"：前者不该改，后者是显式清空。
// 这是 PATCH 语义，用零值判断会把"清空简介"当成"不改简介"。
type ProfileUpdate struct {
	Username *string
	Nickname *string
	Avatar   *string
	Abstract *string
}

// IsEmpty 判断是否什么都没改（可以直接跳过写库）。
func (u ProfileUpdate) IsEmpty() bool {
	return u.Username == nil && u.Nickname == nil && u.Avatar == nil && u.Abstract == nil
}

// Validate 校验本次更新里出现的字段。
func (u ProfileUpdate) Validate() error {
	if u.Username != nil {
		if err := ValidateUsername(*u.Username); err != nil {
			return err
		}
	}
	if u.Nickname != nil {
		if err := ValidateNickname(*u.Nickname); err != nil {
			return err
		}
	}
	if u.Abstract != nil {
		if err := ValidateAbstract(*u.Abstract); err != nil {
			return err
		}
	}
	return nil
}

// WantsUsernameChange 判断本次是否真的要改用户名。
//
// 前端提交整份表单，"用户名没变"也是常态，不能因此报冷却期错误。
func (u ProfileUpdate) WantsUsernameChange(current User) (string, bool) {
	if u.Username == nil {
		return "", false
	}
	next := strings.TrimSpace(*u.Username)
	if next == current.Username {
		return "", false
	}
	return next, true
}

// CanUpdateUsername 判断能否改用户名：本人或管理员，且过了冷却期。
//
// 冷却期对管理员同样生效：这是产品规则，不是权限规则，管理员没有理由绕过。
func CanUpdateUsername(target User, settings Settings, viewer Viewer, now time.Time) error {
	if err := CanUpdateProfile(target, viewer); err != nil {
		return err
	}
	if settings.UpdatedUsernameAt == nil {
		return nil
	}
	if now.Sub(*settings.UpdatedUsernameAt) < UsernameCooldown {
		return apperr.New(apperr.CodeUserUsernameCooldown)
	}
	return nil
}

// ApplyProfileUpdate 把更新作用到实体上，返回被改动的字段名。
//
// 返回字段名而不是直接写库：调用方据此决定要不要同步冗余快照
// （昵称/头像变了才需要同步到评论、关注等表）。
func (u *User) ApplyProfileUpdate(update ProfileUpdate) []string {
	var changed []string
	if update.Username != nil {
		next := strings.TrimSpace(*update.Username)
		if next != u.Username {
			u.Username = next
			changed = append(changed, "username")
		}
	}
	if update.Nickname != nil {
		next := normalizeNickname(*update.Nickname)
		if next != u.Nickname {
			u.Nickname = next
			changed = append(changed, "nickname")
		}
	}
	if update.Avatar != nil {
		if *update.Avatar != u.Avatar {
			u.Avatar = *update.Avatar
			changed = append(changed, "avatar")
		}
	}
	if update.Abstract != nil {
		if *update.Abstract != u.Abstract {
			u.Abstract = *update.Abstract
			changed = append(changed, "abstract")
		}
	}
	return changed
}

// NeedsDisplaySnapshotSync 判断本次改动是否需要同步其他表的冗余展示字段。
func NeedsDisplaySnapshotSync(changed []string) bool {
	for _, f := range changed {
		switch f {
		case "nickname", "avatar", "abstract":
			return true
		}
	}
	return false
}
