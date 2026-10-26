package app

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/domain"
)

// 本文件是主页展示与管理端用例：主页基础信息、资料+偏好合并更新、
// 管理员改用户信息、管理端用户列表。

// BaseInfo 返回主页展示信息（含关注关系与主页浏览计数）。
//
// 浏览计数只在"登录用户看别人的主页"时 +1，去重由外部 HomeViewRecorder 负责；
// 关系与计数都是展示增强，失败不影响主页本身能不能打开。
func (s *Service) BaseInfo(ctx context.Context, viewer, target domain.UserID) (*PublicProfile, error) {
	profile, err := s.PublicProfile(ctx, target)
	if err != nil {
		return nil, err
	}
	if s.enricher != nil && profile.Avatar != "" {
		if id, ok, e := s.enricher.AvatarIDByURL(ctx, profile.Avatar); e == nil && ok {
			profile.AvatarImageID = &id
		}
	}
	if viewer.IsZero() {
		return profile, nil
	}
	if s.relation != nil {
		if rel, e := s.relation.Relation(ctx, viewer, target); e == nil {
			profile.Relation = rel
		}
	}
	if viewer != target && s.homeView != nil {
		if counted, e := s.homeView.Record(ctx, target, viewer); e == nil && counted {
			profile.Stats.ViewCount++
		}
	}
	return profile, nil
}

// UpdateInfoInput 是"资料 + 偏好"合并更新的输入。
type UpdateInfoInput struct {
	Profile       domain.ProfileUpdate
	AvatarImageID *ctype.ID
	Settings      SettingsPatch
}

// UpdateInfo 一次更新资料与偏好。
//
// 顺序：先把头像图片 ID 换成 URL，再走资料更新（权限/格式/重名/冷却都在里面），
// 最后写偏好。资料失败就整体失败，不会出现"昵称改了偏好没改"的半截结果。
func (s *Service) UpdateInfo(ctx context.Context, viewer domain.Viewer, target domain.UserID, in UpdateInfoInput) (*UpdateResult, error) {
	if in.AvatarImageID != nil && s.enricher != nil {
		url, err := s.enricher.AvatarURLByID(ctx, *in.AvatarImageID)
		if err != nil {
			return nil, err
		}
		in.Profile.Avatar = &url
	}
	res, err := s.Update(ctx, viewer, target, in.Profile)
	if err != nil {
		return nil, err
	}
	if err := s.UpdateSettings(ctx, viewer, target, in.Settings); err != nil {
		return nil, err
	}
	return res, nil
}

// AdminPatchInput 是管理员改用户信息的输入。
type AdminPatchInput struct {
	Username      *string
	Nickname      *string
	Abstract      *string
	AvatarImageID *ctype.ID
	Role          *domain.Role
	Status        *domain.Status
}

// AdminUpdate 管理员修改指定用户信息。
//
// 角色/状态一旦变化就把令牌版本 +1 并吊销全部会话：否则被降权/封禁的人
// 手里那张未过期的 access token 还能继续用。
func (s *Service) AdminUpdate(ctx context.Context, viewer domain.Viewer, target domain.UserID, in AdminPatchInput) error {
	if err := domain.CanManageUsers(viewer); err != nil {
		return err
	}
	if in.Username != nil {
		if err := domain.ValidateUsername(*in.Username); err != nil {
			return err
		}
		taken, err := s.users.ExistsUsername(ctx, *in.Username, target)
		if err != nil {
			return err
		}
		if taken {
			return apperr.New(apperr.CodeUserUsernameTaken)
		}
	}
	if in.Nickname != nil {
		if err := domain.ValidateNickname(*in.Nickname); err != nil {
			return err
		}
	}
	if in.Status != nil && viewer.ID == target {
		return apperr.New(apperr.CodeUserStatusSelfLocked)
	}

	patch := domain.AdminPatch{
		Username: in.Username,
		Nickname: in.Nickname,
		Abstract: in.Abstract,
		Role:     in.Role,
		Status:   in.Status,
	}
	if in.AvatarImageID != nil && s.enricher != nil {
		url, err := s.enricher.AvatarURLByID(ctx, *in.AvatarImageID)
		if err != nil {
			return err
		}
		patch.AvatarURL = &url
	}

	if err := s.users.UpdateAdmin(ctx, target, patch); err != nil {
		return err
	}
	if in.Nickname != nil || patch.AvatarURL != nil || in.Abstract != nil {
		if s.snapshots != nil {
			if err := s.snapshots.SyncDisplay(ctx, target); err != nil {
				s.logError("同步用户展示快照失败: user=%s err=%v", target.String(), err)
			}
		}
	}
	if in.Role != nil || in.Status != nil {
		if err := s.users.BumpTokenVersion(ctx, target); err != nil {
			return err
		}
		if s.sessions != nil {
			if err := s.sessions.RevokeAll(ctx, target); err != nil {
				s.logError("吊销用户会话失败: user=%s err=%v", target.String(), err)
			}
		}
	}
	return nil
}

// AdminUserItem 是管理端用户列表的一行。
type AdminUserItem struct {
	ID          ctype.ID
	Nickname    string
	Avatar      string
	Username    string
	CreatedAt   time.Time
	IP          string
	Addr        string
	LastLoginAt time.Time
}

// AdminList 分页查询用户，并附带最后登录信息。
func (s *Service) AdminList(ctx context.Context, viewer domain.Viewer, q domain.Query) ([]AdminUserItem, int64, error) {
	page, err := s.List(ctx, viewer, q)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uint64, 0, len(page.Items))
	for _, u := range page.Items {
		ids = append(ids, u.ID.Uint64())
	}
	var last map[uint64]LastLogin
	if s.lastLogin != nil && len(ids) > 0 {
		if got, e := s.lastLogin.Latest(ctx, ids); e == nil {
			last = got
		}
	}
	items := make([]AdminUserItem, 0, len(page.Items))
	for _, u := range page.Items {
		item := AdminUserItem{
			ID:        u.ID,
			Nickname:  u.Nickname,
			Avatar:    u.Avatar,
			Username:  u.Username,
			CreatedAt: u.CreatedAt,
		}
		if l, ok := last[u.ID.Uint64()]; ok {
			item.IP = l.IP
			item.Addr = l.Addr
			item.LastLoginAt = l.At
		}
		items = append(items, item)
	}
	return items, page.Total, nil
}
