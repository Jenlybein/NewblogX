package domain

import (
	"fmt"
	"regexp"
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

// 禁言时长的边界（票 399 / ADR-0032 决策 8）。
//
// 上限 365 天不是拍脑袋：禁言是"带时长的临时处罚"，再长就该走封禁（账号禁用），
// 而封禁不经过禁言通道。下限 1 天避免"0 天禁言"这种看起来执行了、实际没生效的输入。
const (
	// DefaultMuteDays 是未指定时长时的默认天数（spec 的"默认 7 天"）。
	DefaultMuteDays = 7
	// MinMuteDays 是最短禁言天数。
	MinMuteDays = 1
	// MaxMuteDays 是最长禁言天数。
	MaxMuteDays = 365
)

// NormalizeMuteDays 校验并收敛禁言天数：0 表示"没传"，按默认 7 天；越界报错。
//
// 缺省值放在领域层而不是 HTTP 层：风险页的单条处置与批量处置都必须拿到同一份
func NormalizeMuteDays(raw int) (int, error) {
	if raw == 0 {
		return DefaultMuteDays, nil
	}
	if raw < MinMuteDays || raw > MaxMuteDays {
		return 0, apperr.New(apperr.CodeUserMuteDaysInvalid)
	}
	return raw, nil
}

// MuteUntilAt 返回从 now 起 days 天的禁言到期时间。
func MuteUntilAt(now time.Time, days int) time.Time {
	return now.Add(time.Duration(days) * 24 * time.Hour)
}

// MuteError 是"账号处于禁言期，发布类动作被拒"。
//
// 为什么不用 apperr.New 的固定文案：用户要知道**什么时候恢复**，只给一句
// "已被禁言"会让人反复重试或去问管理员。错误码仍然只有一个
// （apperr.CodeUserMuted），前端按码分支、按文案展示。
type MuteError struct {
	// Until 是禁言到期时间。
	Until time.Time
	// Now 是判定时的时间（文案里算剩余天数用），零值时按 Until 前 1 秒处理。
	Now time.Time
}

// NewMuteError 构造禁言错误。
func NewMuteError(until, now time.Time) *MuteError {
	return &MuteError{Until: until, Now: now}
}

// Error 返回面向用户的文案（含到期时间与剩余天数）。
func (e *MuteError) Error() string {
	remaining := e.remainingDays()
	return fmt.Sprintf(
		"账号已被禁言，禁言至 %s（还有 %d 天），期间不能发布、评论、私信或修改资料",
		e.Until.Format("2006-01-02 15:04"),
		remaining,
	)
}

// Unwrap 让 errors.Is / apperr.CodeOf 能拿到错误码（文案由 Error 覆盖）。
func (e *MuteError) Unwrap() error { return apperr.New(apperr.CodeUserMuted) }

// UserMessage 让共享响应层用这句动态文案，而不是消息表里那句静态的。
//
// 禁言的静态文案只有「账号已被禁言」，而「到哪天、还有几天」正是用户唯一想知道的。
// res.Fail 只在 ProtocolValidation 上认这个方法，所以它不会把服务异常的内部细节
// 带进响应（这条通道不给别的错误用）。
func (e *MuteError) UserMessage() string { return e.Error() }

func (e *MuteError) remainingDays() int {
	now := e.Now
	if now.IsZero() {
		now = e.Until.Add(-time.Second)
	}
	remaining := e.Until.Sub(now)
	if remaining <= 0 {
		return 0
	}
	days := int(remaining/(24*time.Hour)) + 1
	if remaining%(24*time.Hour) == 0 {
		days = int(remaining / (24 * time.Hour))
	}
	return days
}

// CanPublish 判断账号此刻能否执行发布类动作（票 399 / ADR-0032 决策 8）。
//
// 生效范围只有发布类动作：文章创建 / 投稿 / 更新、评论、私信、昵称与简介修改。
// 登录、浏览、点赞、收藏一律不看这一条 —— 禁言的目的是"停止输出"，
// 不是"把人赶出站点"。
//
// 账号状态（禁用 / 封禁）优先于禁言：一个被封禁的账号先拿到"账号已被封禁"，
// 而不是"你被禁言到某天"（后者会让人误以为到期就能恢复）。
func CanPublish(u User, now time.Time) error {
	if err := CanLogin(u); err != nil {
		return err
	}
	if u.IsMuted(now) {
		return NewMuteError(*u.MuteUntil, now)
	}
	return nil
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

// minPhonePattern 是中国大陆手机号的形状校验：11 位、以 1 开头、第二位 3-9。
//
// 只校验"形状"，不做归属地/在网校验 —— 真实有效性由短信验证码证明：
// 收不到码就绑不上，比任何号段库都准。
var minPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

// ValidatePhone 校验手机号形状。
func ValidatePhone(s string) error {
	if !minPhonePattern.MatchString(strings.TrimSpace(s)) {
		return apperr.New(apperr.CodeUserPhoneInvalid)
	}
	return nil
}

// MaskPhone 把手机号打成 138****8000 的形状，用于默认昵称与后台展示。
//
// 默认昵称用打码号而不是完整号：昵称会出现在评论、关注列表等公开位置，
// 直接放完整手机号等于替用户公开了隐私。
func MaskPhone(phone string) string {
	s := strings.TrimSpace(phone)
	if len(s) != 11 {
		// 非 11 位的输入（历史脏数据）不猜结构，整体打码。
		return "***"
	}
	return s[:3] + "****" + s[7:]
}

// MinPasswordLen 是密码最小长度。
//
// 与认证模块保持同一口径：只约束长度，不做复杂度要求。复杂度规则会逼用户写
// "Passw0rd!" 这类好猜又好忘的组合，长度才是更有效的约束。
const MinPasswordLen = 8

// ValidatePassword 校验密码长度。
func ValidatePassword(s string) error {
	if s == "" {
		return apperr.New(apperr.CodeUserPasswordRequired)
	}
	if len(s) < MinPasswordLen {
		return apperr.New(apperr.CodeUserPasswordTooShort)
	}
	return nil
}

