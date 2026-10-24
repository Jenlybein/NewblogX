// Package mysql 是用户模块的 MySQL 适配器。
//
// 只有这一层知道 GORM、表名和列名。上层拿到的错误一律是 apperr 错误码，
// 不会看到 "record not found" 这类存储细节。
package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"

	"gorm.io/gorm"
)

// Repo 实现用户模块的全部持久化端口。
//
// 一个结构体实现五个接口（Repository / SettingsRepository / StatsRepository /
// DisplayReader / SnapshotSyncer）：它们操作的是同一批表，拆成五个类型再互相
// 注入只会增加装配噪音。
type Repo struct {
	db *gorm.DB
}

// 编译期断言：端口必须全部被满足。
var (
	_ domain.Repository    = (*Repo)(nil)
	_ domain.DisplayReader = (*Repo)(nil)
)

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// NewSettings 构造偏好仓储（同名方法 Find 与统计仓储冲突，只能拆成不同类型）。
func NewSettings(db *gorm.DB) *SettingsRepo { return &SettingsRepo{db: db} }

// NewStats 构造统计仓储。
func NewStats(db *gorm.DB) *StatsRepo { return &StatsRepo{db: db} }

// ErrNilDB 表示构造时没给数据库连接。
var ErrNilDB = errors.New("user/mysql: nil *gorm.DB")

// FindByID 实现 domain.Repository。
func (r *Repo) FindByID(ctx context.Context, id domain.UserID) (domain.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).Take(&row, "id = ?", id.Uint64()).Error
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toUser(row), nil
}

// FindByUsername 实现 domain.Repository。
func (r *Repo) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).Take(&row, "username = ?", username).Error
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toUser(row), nil
}

// FindByEmail 实现 domain.Repository。
func (r *Repo) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).Take(&row, "email = ?", email).Error
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toUser(row), nil
}

// FindByOpenID 实现 domain.Repository。
func (r *Repo) FindByOpenID(ctx context.Context, openID string) (domain.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).Take(&row, "open_id = ?", openID).Error
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toUser(row), nil
}

// ExistsUsername 实现 domain.Repository。
func (r *Repo) ExistsUsername(ctx context.Context, username string, exclude domain.UserID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&UserRow{}).Where("username = ?", username)
	if !exclude.IsZero() {
		q = q.Where("id <> ?", exclude.Uint64())
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, translate(err)
	}
	return count > 0, nil
}

// ExistsEmail 实现 domain.Repository。
func (r *Repo) ExistsEmail(ctx context.Context, email string, exclude domain.UserID) (bool, error) {
	q := r.db.WithContext(ctx).Model(&UserRow{}).Where("email = ?", email)
	if !exclude.IsZero() {
		q = q.Where("id <> ?", exclude.Uint64())
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, translate(err)
	}
	return count > 0, nil
}

// Create 实现 domain.Repository。
func (r *Repo) Create(ctx context.Context, user *domain.User) error {
	row := fromUser(*user)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return translate(err)
	}
	user.ID = domain.UserID(row.ID)
	user.CreatedAt = row.CreatedAt
	return nil
}

// UpdateProfile 实现 domain.Repository。
//
// 只写调用方真正改过的列：整行 Save 会把并发修改的其他字段覆盖掉。
func (r *Repo) UpdateProfile(ctx context.Context, id domain.UserID, update domain.ProfileUpdate, at *time.Time) error {
	cols := map[string]any{}
	if update.Username != nil {
		cols["username"] = strings.TrimSpace(*update.Username)
	}
	if update.Nickname != nil {
		cols["nickname"] = strings.TrimSpace(*update.Nickname)
	}
	if update.Avatar != nil {
		cols["avatar"] = *update.Avatar
	}
	if update.Abstract != nil {
		cols["abstract"] = *update.Abstract
	}
	if len(cols) == 0 && at == nil {
		return nil
	}
	if len(cols) > 0 {
		res := r.db.WithContext(ctx).Model(&UserRow{}).Where("id = ?", id.Uint64()).Updates(cols)
		if res.Error != nil {
			return translate(res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.New(apperr.CodeUserNotFound)
		}
	}
	if at != nil {
		// 冷却时间戳在 user_conf 表；不存在就先补一行默认值，
		// 否则老账号（没有 conf 行）改用户名会静默失败。
		if err := initSettingsDefaults(ctx, r.db, id); err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Model(&UserConfRow{}).
			Where("user_id = ?", id.Uint64()).
			Update("updated_username_date", at).Error; err != nil {
			return translate(err)
		}
	}
	return nil
}

// UpdateStatus 实现 domain.Repository。
func (r *Repo) UpdateStatus(ctx context.Context, id domain.UserID, status domain.Status) error {
	res := r.db.WithContext(ctx).Model(&UserRow{}).
		Where("id = ?", id.Uint64()).
		Update("status", int8(status))
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeUserNotFound)
	}
	return nil
}

// UpdatePassword 实现 domain.Repository。
//
// 同时写令牌版本与改密时间：版本 +1 让所有旧访问令牌立即失效，
// 改密时间用于前端展示"上次修改密码"。
func (r *Repo) UpdatePassword(ctx context.Context, id domain.UserID, hash string, tokenVersion uint32) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&UserRow{}).
		Where("id = ?", id.Uint64()).
		Updates(map[string]any{
			"password":                 hash,
			"token_version":            tokenVersion,
			"last_password_changed_at": now,
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeUserNotFound)
	}
	return nil
}

// UpdateEmail 实现 domain.Repository。
func (r *Repo) UpdateEmail(ctx context.Context, id domain.UserID, email string) error {
	res := r.db.WithContext(ctx).Model(&UserRow{}).
		Where("id = ?", id.Uint64()).
		Update("email", email)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeUserNotFound)
	}
	return nil
}

// List 实现 domain.Repository。
func (r *Repo) List(ctx context.Context, q domain.Query) (domain.Page, error) {
	q = q.Normalize()
	base := r.db.WithContext(ctx).Model(&UserRow{})
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + kw + "%"
		base = base.Where("username LIKE ? OR nickname LIKE ?", like, like)
	}
	if q.Status != nil {
		base = base.Where("status = ?", int8(*q.Status))
	}
	if q.Role != nil {
		base = base.Where("role = ?", int8(*q.Role))
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return domain.Page{}, translate(err)
	}

	var rows []UserRow
	if err := base.Order("id DESC").Limit(q.Size).Offset(q.Offset()).Find(&rows).Error; err != nil {
		return domain.Page{}, translate(err)
	}
	items := make([]domain.User, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUser(row))
	}
	return domain.Page{Items: items, Total: total}, nil
}

// LoadDisplays 实现 domain.DisplayReader。
func (r *Repo) LoadDisplays(ctx context.Context, ids []domain.UserID) (map[domain.UserID]domain.Display, error) {
	out := make(map[domain.UserID]domain.Display, len(ids))
	raw := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		if _, dup := seen[id.Uint64()]; dup {
			continue
		}
		seen[id.Uint64()] = struct{}{}
		raw = append(raw, id.Uint64())
	}
	if len(raw) == 0 {
		return out, nil
	}

	var rows []UserRow
	if err := r.db.WithContext(ctx).
		Select("id", "nickname", "avatar", "abstract", "role").
		Where("id IN ?", raw).Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	for _, row := range rows {
		out[domain.UserID(row.ID)] = domain.Display{
			ID:       domain.UserID(row.ID),
			Nickname: row.Nickname,
			Avatar:   row.Avatar,
			Abstract: row.Abstract,
			Role:     domain.Role(row.Role),
		}
	}
	return out, nil
}

// translate 把 GORM 错误翻成业务错误码。
//
// 上层不该看到 "record not found"：那是存储细节，也无法据此给用户提示。
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeUserNotFound)
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return apperr.Wrap(apperr.CodeUserUsernameTaken, err)
	}
	return err
}

func toUser(row UserRow) domain.User {
	return domain.User{
		ID:             domain.UserID(row.ID),
		CreatedAt:      row.CreatedAt,
		Username:       row.Username,
		Nickname:       row.Nickname,
		Avatar:         row.Avatar,
		Abstract:       row.Abstract,
		Email:          row.Email,
		PasswordHash:   row.Password,
		OpenID:         row.OpenID,
		Status:         domain.Status(row.Status),
		Role:           domain.Role(row.Role),
		RegisterSource: domain.RegisterSource(row.RegisterSource),
		TokenVersion:   row.TokenVersion,
		IP:             row.IP,
		Addr:           row.Addr,
	}
}

func fromUser(u domain.User) UserRow {
	return UserRow{
		BaseRow:        BaseRow{ID: u.ID.Uint64(), CreatedAt: u.CreatedAt},
		Username:       u.Username,
		Nickname:       u.Nickname,
		Avatar:         u.Avatar,
		Abstract:       u.Abstract,
		RegisterSource: int8(u.RegisterSource),
		Password:       u.PasswordHash,
		Email:          u.Email,
		OpenID:         u.OpenID,
		Status:         int8(u.Status),
		TokenVersion:   u.TokenVersion,
		Role:           int8(u.Role),
		IP:             u.IP,
		Addr:           u.Addr,
	}
}

func toSettings(row UserConfRow) domain.Settings {
	return domain.Settings{
		UserID:                   domain.UserID(row.UserID),
		LikeTagIDs:               row.LikeTags,
		UpdatedUsernameAt:        row.UpdatedUsernameDate,
		FavoritesVisibility:      row.FavoritesVisibility,
		FollowVisibility:         row.FollowVisibility,
		FansVisibility:           row.FansVisibility,
		HomeStyleID:              row.HomeStyleID,
		DiggNoticeEnabled:        row.DiggNoticeEnabled,
		CommentNoticeEnabled:     row.CommentNoticeEnabled,
		FavorNoticeEnabled:       row.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: row.PrivateChatNoticeEnabled,
		StrangerChatEnabled:      row.StrangerChatEnabled,
	}
}

func fromSettings(s domain.Settings) UserConfRow {
	return UserConfRow{
		UserID:                   s.UserID.Uint64(),
		LikeTags:                 s.LikeTagIDs,
		UpdatedUsernameDate:      s.UpdatedUsernameAt,
		FavoritesVisibility:      s.FavoritesVisibility,
		FollowVisibility:         s.FollowVisibility,
		FansVisibility:           s.FansVisibility,
		HomeStyleID:              s.HomeStyleID,
		DiggNoticeEnabled:        s.DiggNoticeEnabled,
		CommentNoticeEnabled:     s.CommentNoticeEnabled,
		FavorNoticeEnabled:       s.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: s.PrivateChatNoticeEnabled,
		StrangerChatEnabled:      s.StrangerChatEnabled,
	}
}
