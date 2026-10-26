package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/domain"
)

// PublicProfile 是公开主页展示的信息（不含邮箱、通知开关等私密项）。
type PublicProfile struct {
	ID           ctype.ID
	Username     string
	Nickname     string
	Avatar       string
	Abstract     string
	CodeAge      int
	Stats        domain.Stats
	FavoritesVis bool
	FollowVis    bool
	FansVis      bool

	// 以下字段是主页展示需要的补充信息。
	AvatarImageID *ctype.ID
	HomeStyleID   ctype.ID
	Place         string
	Relation      int8
}

// Detail 返回当前登录用户的完整资料。
//
// 只能看自己：这个接口带邮箱和通知开关，用他人的 ID 调用等于泄露。
func (s *Service) Detail(ctx context.Context, viewer domain.Viewer) (*DetailView, error) {
	if viewer.IsZero() {
		return nil, apperr.New(apperr.CodeUserForbidden)
	}

	u, err := s.users.FindByID(ctx, viewer.ID)
	if err != nil {
		return nil, err
	}
	settings, err := s.settings.Find(ctx, viewer.ID)
	if err != nil {
		return nil, err
	}

	view := &DetailView{
		ID:                       u.ID,
		CreatedAt:                u.CreatedAt,
		Username:                 u.Username,
		Nickname:                 u.Nickname,
		Avatar:                   u.Avatar,
		Abstract:                 u.Abstract,
		Email:                    u.Email,
		HasPassword:              u.HasPassword(),
		RegisterSource:           int8(u.RegisterSource),
		CodeAge:                  u.CodeAge(s.clock.Now()),
		LikeTagIDs:               settings.LikeTagIDs,
		LikeTagItems:             []LikeTagItem{},
		UpdatedUsernameDate:      settings.UpdatedUsernameAt,
		FavoritesVisibility:      settings.FavoritesVisibility,
		FollowVisibility:         settings.FollowVisibility,
		FansVisibility:           settings.FansVisibility,
		HomeStyleID:              settings.HomeStyleID,
		DiggNoticeEnabled:        settings.DiggNoticeEnabled,
		CommentNoticeEnabled:     settings.CommentNoticeEnabled,
		FavorNoticeEnabled:       settings.FavorNoticeEnabled,
		PrivateChatNoticeEnabled: settings.PrivateChatNoticeEnabled,
		StrangerChatEnabled:      settings.StrangerChatEnabled,
	}

	// 头像图片 ID 是"锦上添花"：查不到就不给，不能因此让整个详情接口失败。
	if s.enricher != nil && u.Avatar != "" {
		if id, ok, err := s.enricher.AvatarIDByURL(ctx, u.Avatar); err == nil && ok {
			view.AvatarImageID = &id
		}
	}

	// 标签标题同理：标签表在 article 模块，读不到就只返回 ID 列表。
	if s.enricher != nil && len(settings.LikeTagIDs) > 0 {
		if titles, err := s.enricher.TagTitles(ctx, settings.LikeTagIDs); err == nil {
			view.LikeTagItems = orderTagItems(settings.LikeTagIDs, titles)
		}
	}

	return view, nil
}

// PublicProfile 返回用户公开主页信息。
func (s *Service) PublicProfile(ctx context.Context, target domain.UserID) (*PublicProfile, error) {
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := domain.CanViewProfile(u); err != nil {
		return nil, err
	}
	settings, err := s.settings.Find(ctx, target)
	if err != nil {
		return nil, err
	}
	stats, err := s.stats.Find(ctx, target)
	if err != nil {
		return nil, err
	}
	return &PublicProfile{
		ID:           u.ID,
		Username:     u.Username,
		Nickname:     u.Nickname,
		Avatar:       u.Avatar,
		Abstract:     u.Abstract,
		CodeAge:      u.CodeAge(s.clock.Now()),
		Stats:        stats,
		FavoritesVis: settings.FavoritesVisibility,
		FollowVis:    settings.FollowVisibility,
		FansVis:      settings.FansVisibility,
		HomeStyleID:  settings.HomeStyleID,
		Place:        u.Addr,
	}, nil
}

// Update 更新用户资料。
//
// 顺序有讲究：先判权限 → 再校验格式 → 再查重 → 最后落库 + 同步快照。
// 快照同步失败只记日志：资料本身已经改成功了，为了一条冗余字段回滚主数据不值得。
func (s *Service) Update(ctx context.Context, viewer domain.Viewer, target domain.UserID, update domain.ProfileUpdate) (*UpdateResult, error) {
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := domain.CanUpdateProfile(u, viewer); err != nil {
		return nil, err
	}
	if err := update.Validate(); err != nil {
		return nil, err
	}
	if update.IsEmpty() {
		return &UpdateResult{User: u}, nil
	}

	var usernameAt *time.Time
	if next, wants := update.WantsUsernameChange(u); wants {
		settings, err := s.settings.Find(ctx, target)
		if err != nil {
			return nil, err
		}
		now := s.clock.Now()
		if err := domain.CanUpdateUsername(u, settings, viewer, now); err != nil {
			return nil, err
		}
		taken, err := s.users.ExistsUsername(ctx, next, target)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, apperr.New(apperr.CodeUserUsernameTaken)
		}
		usernameAt = &now
	}

	changed := u.ApplyProfileUpdate(update)
	if len(changed) == 0 && usernameAt == nil {
		return &UpdateResult{User: u}, nil
	}

	if err := s.users.UpdateProfile(ctx, target, update, usernameAt); err != nil {
		return nil, err
	}
	if usernameAt != nil {
		// 冷却时间戳落在 user_conf 表，与资料表分属两张表，必须单独写。
		settings, err := s.settings.Find(ctx, target)
		if err == nil {
			settings.UpdatedUsernameAt = usernameAt
			if err := s.settings.Save(ctx, settings); err != nil {
				s.logError("保存用户名冷却时间戳失败: user=%s err=%v", target.String(), err)
			}
		}
	}

	res := &UpdateResult{User: u, Changed: changed}
	if domain.NeedsDisplaySnapshotSync(changed) && s.snapshots != nil {
		if err := s.snapshots.SyncDisplay(ctx, target); err != nil {
			s.logError("同步用户展示快照失败: user=%s err=%v", target.String(), err)
		} else {
			res.SnapshotSynced = true
		}
	}
	return res, nil
}

// UpdateSettings 更新偏好设置。
func (s *Service) UpdateSettings(ctx context.Context, viewer domain.Viewer, target domain.UserID, patch SettingsPatch) error {
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return err
	}
	if err := domain.CanUpdateProfile(u, viewer); err != nil {
		return err
	}
	if patch.IsEmpty() {
		return nil
	}
	if patch.LikeTagIDs != nil && s.tags != nil {
		if err := s.tags.Validate(ctx, *patch.LikeTagIDs); err != nil {
			return err
		}
	}
	current, err := s.settings.Find(ctx, target)
	if err != nil {
		return err
	}
	return s.settings.Save(ctx, patch.Apply(current))
}

// List 分页查询用户（管理端）。
func (s *Service) List(ctx context.Context, viewer domain.Viewer, q domain.Query) (domain.Page, error) {
	if err := domain.CanManageUsers(viewer); err != nil {
		return domain.Page{}, err
	}
	return s.users.List(ctx, q.Normalize())
}

// ChangeStatus 修改账号状态（管理端）。
func (s *Service) ChangeStatus(ctx context.Context, viewer domain.Viewer, target domain.UserID, status domain.Status) error {
	if err := domain.ValidateStatus(status); err != nil {
		return err
	}
	u, err := s.users.FindByID(ctx, target)
	if err != nil {
		return err
	}
	if err := domain.CanChangeStatus(u, viewer); err != nil {
		return err
	}
	if u.Status == status {
		return nil
	}
	return s.users.UpdateStatus(ctx, target, status)
}

// Create 创建用户（管理端）。
//
// 建档三件套（用户行 + 偏好行 + 统计行）必须全部成功，否则会出现
// "能登录但主页 500"的半成品账号，所以这里不吞错误。
func (s *Service) Create(ctx context.Context, viewer domain.Viewer, in CreateInput) (domain.User, error) {
	if err := domain.CanManageUsers(viewer); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidateUsername(in.Username); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidateNickname(in.Nickname); err != nil {
		return domain.User{}, err
	}
	taken, err := s.users.ExistsUsername(ctx, in.Username, "")
	if err != nil {
		return domain.User{}, err
	}
	if taken {
		return domain.User{}, apperr.New(apperr.CodeUserUsernameTaken)
	}
	if in.Email != nil && *in.Email != "" {
		emailTaken, err := s.users.ExistsEmail(ctx, *in.Email, "")
		if err != nil {
			return domain.User{}, err
		}
		if emailTaken {
			return domain.User{}, apperr.New(apperr.CodeUserEmailTaken)
		}
	}
	if in.Password == "" {
		return domain.User{}, apperr.New(apperr.CodeUserPasswordRequired)
	}
	if s.hasher == nil {
		return domain.User{}, apperr.New(apperr.CodeUserPasswordRequired)
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, err
	}

	status := in.Status
	if status == 0 {
		status = domain.StatusActive
	}
	if err := domain.ValidateStatus(status); err != nil {
		return domain.User{}, err
	}
	role := in.Role
	if role == 0 {
		role = domain.RoleUser
	}

	u := domain.User{
		Username:       in.Username,
		Nickname:       in.Nickname,
		Email:          in.Email,
		PasswordHash:   hash,
		Status:         status,
		Role:           role,
		RegisterSource: domain.SourceAdmin,
		TokenVersion:   1,
		IP:             in.IP,
		Addr:           in.Addr,
		CreatedAt:      s.clock.Now(),
	}
	if err := s.users.Create(ctx, &u); err != nil {
		return domain.User{}, err
	}
	if err := s.settings.InitDefaults(ctx, u.ID); err != nil {
		return domain.User{}, err
	}
	if err := s.stats.Init(ctx, u.ID); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// AddStats 按增量累加用户统计。
//
// 供 article（文章数、被访问数）、social（粉丝/关注数）等模块调用。
// 走增量而不是覆盖，避免并发下的丢失更新。
func (s *Service) AddStats(ctx context.Context, target domain.UserID, delta domain.StatsDelta) error {
	if target.IsZero() || delta.IsZero() {
		return nil
	}
	return s.stats.Add(ctx, target, delta)
}

func (s *Service) logError(format string, args ...any) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}

// orderTagItems 按传入顺序组装标签项，保证前端展示顺序稳定。
func orderTagItems(ids []ctype.ID, titles map[ctype.ID]string) []LikeTagItem {
	out := make([]LikeTagItem, 0, len(ids))
	for _, id := range ids {
		title, ok := titles[id]
		if !ok {
			continue
		}
		out = append(out, LikeTagItem{ID: id, Title: title})
	}
	return out
}
