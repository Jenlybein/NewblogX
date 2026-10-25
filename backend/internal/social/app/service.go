// Package app 是社交（关注关系）模块的用例层。
package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/social/domain"
)

// Service 是关注关系应用服务。
type Service struct {
	repo    domain.Repository
	stats   domain.UserStats
	users   domain.UserDisplay
	privacy domain.Privacy
	logger  domain.Logger
}

// New 构造服务。
func New(repo domain.Repository, stats domain.UserStats, users domain.UserDisplay, privacy domain.Privacy, logger domain.Logger) *Service {
	if repo == nil {
		panic("social/app: 关注仓储未注入")
	}
	if users == nil {
		panic("social/app: 用户展示读未注入")
	}
	return &Service{repo: repo, stats: stats, users: users, privacy: privacy, logger: logger}
}

// Follow 关注目标用户。
func (s *Service) Follow(ctx context.Context, viewerID, targetID uint64) error {
	if viewerID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if viewerID == targetID {
		return apperr.New(apperr.CodeFollowSelf)
	}

	displays, err := s.users.Displays(ctx, []uint64{viewerID, targetID})
	if err != nil {
		return err
	}
	followed, ok := displays[targetID]
	if !ok {
		return apperr.New(apperr.CodeFollowTargetNotFound)
	}
	fans := displays[viewerID]

	created, err := s.repo.Follow(ctx, &domain.Follow{
		FollowedUserID:   targetID,
		FollowedNickname: followed.Nickname,
		FollowedAvatar:   followed.Avatar,
		FollowedAbstract: followed.Abstract,
		FansUserID:       viewerID,
		FansNickname:     fans.Nickname,
		FansAvatar:       fans.Avatar,
		FansAbstract:     fans.Abstract,
	})
	if err != nil {
		return err
	}
	if !created {
		return apperr.New(apperr.CodeFollowDuplicate)
	}

	// 统计是副作用：失败只记日志，关注关系本身已经成立。
	if s.stats != nil {
		if err := s.stats.ApplyFollowDelta(ctx, viewerID, targetID, 1); err != nil {
			s.logError("更新关注统计失败: fans=%d followed=%d err=%v", viewerID, targetID, err)
		}
	}
	return nil
}

// Unfollow 取消关注。
func (s *Service) Unfollow(ctx context.Context, viewerID, targetID uint64) error {
	if viewerID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if viewerID == targetID {
		return apperr.New(apperr.CodeFollowSelf)
	}

	removed, err := s.repo.Unfollow(ctx, targetID, viewerID)
	if err != nil {
		return err
	}
	if !removed {
		return apperr.New(apperr.CodeFollowNotFollowing)
	}
	if s.stats != nil {
		if err := s.stats.ApplyFollowDelta(ctx, viewerID, targetID, -1); err != nil {
			s.logError("回滚关注统计失败: fans=%d followed=%d err=%v", viewerID, targetID, err)
		}
	}
	return nil
}

// ListFollowing 关注列表。
func (s *Service) ListFollowing(ctx context.Context, viewerID, ownerID, followedFilter uint64, page domain.Page) ([]domain.FollowingItem, bool, error) {
	ownerID = s.resolveOwner(viewerID, ownerID)
	if ownerID == 0 {
		return []domain.FollowingItem{}, false, nil
	}
	if ownerID != viewerID && s.privacy != nil {
		visible, err := s.privacy.FollowVisible(ctx, ownerID)
		if err != nil {
			return nil, false, apperr.New(apperr.CodeFollowConfigMissing)
		}
		if !visible {
			return nil, false, apperr.New(apperr.CodeFollowListPrivate)
		}
	}

	rows, hasMore, err := s.repo.ListFollowing(ctx, ownerID, followedFilter, page)
	if err != nil {
		return nil, false, err
	}
	s.hydrateFollowed(ctx, rows)

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.FollowedUserID)
	}
	relations := s.relationBatch(ctx, viewerID, ids)
	items := make([]domain.FollowingItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.FollowingItem{
			FollowedUserID:   row.FollowedUserID,
			FollowedNickname: row.FollowedNickname,
			FollowedAvatar:   row.FollowedAvatar,
			FollowedAbstract: row.FollowedAbstract,
			FollowTime:       row.CreatedAt,
			Relation:         relations[row.FollowedUserID],
		})
	}
	return items, hasMore, nil
}

// ListFans 粉丝列表。
func (s *Service) ListFans(ctx context.Context, viewerID, ownerID, fansFilter uint64, page domain.Page) ([]domain.FansItem, bool, error) {
	ownerID = s.resolveOwner(viewerID, ownerID)
	if ownerID == 0 {
		return []domain.FansItem{}, false, nil
	}
	if ownerID != viewerID && s.privacy != nil {
		visible, err := s.privacy.FansVisible(ctx, ownerID)
		if err != nil {
			return nil, false, apperr.New(apperr.CodeFollowConfigMissing)
		}
		if !visible {
			return nil, false, apperr.New(apperr.CodeFollowListPrivate)
		}
	}

	rows, hasMore, err := s.repo.ListFans(ctx, ownerID, fansFilter, page)
	if err != nil {
		return nil, false, err
	}
	s.hydrateFans(ctx, rows)

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.FansUserID)
	}
	relations := s.relationBatch(ctx, viewerID, ids)
	items := make([]domain.FansItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.FansItem{
			FansUserID:   row.FansUserID,
			FansNickname: row.FansNickname,
			FansAvatar:   row.FansAvatar,
			FansAbstract: row.FansAbstract,
			FollowTime:   row.CreatedAt,
			Relation:     relations[row.FansUserID],
		})
	}
	return items, hasMore, nil
}

// Relation 计算两人关系（供其他模块复用）。
func (s *Service) Relation(ctx context.Context, viewerID, targetID uint64) (domain.Relation, error) {
	if viewerID == 0 || targetID == 0 {
		return domain.RelationStranger, nil
	}
	got, err := s.repo.RelationBatch(ctx, viewerID, []uint64{targetID})
	if err != nil {
		return domain.RelationStranger, err
	}
	if rel, ok := got[targetID]; ok {
		return rel, nil
	}
	return domain.RelationStranger, nil
}

// RelationBatch 批量计算关系（供评论/用户列表复用）。
func (s *Service) RelationBatch(ctx context.Context, viewerID uint64, ids []uint64) (map[uint64]domain.Relation, error) {
	if viewerID == 0 || len(ids) == 0 {
		out := make(map[uint64]domain.Relation, len(ids))
		for _, id := range ids {
			out[id] = domain.RelationStranger
		}
		return out, nil
	}
	return s.repo.RelationBatch(ctx, viewerID, ids)
}

// resolveOwner 缺省看自己。
func (s *Service) resolveOwner(viewerID, ownerID uint64) uint64 {
	if ownerID != 0 {
		return ownerID
	}
	return viewerID
}

// relationBatch 返回 int8 关系表；匿名时全部陌生人。
func (s *Service) relationBatch(ctx context.Context, viewerID uint64, ids []uint64) map[uint64]int8 {
	out := make(map[uint64]int8, len(ids))
	if viewerID == 0 {
		for _, id := range ids {
			out[id] = domain.RelationStranger.Int8()
		}
		return out
	}
	got, err := s.repo.RelationBatch(ctx, viewerID, ids)
	if err != nil {
		for _, id := range ids {
			out[id] = domain.RelationStranger.Int8()
		}
		return out
	}
	for id, rel := range got {
		out[id] = rel.Int8()
	}
	return out
}

// hydrateFollowed 补齐缺失的被关注者快照。
func (s *Service) hydrateFollowed(ctx context.Context, rows []domain.Follow) {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.FollowedNickname == "" || row.FollowedAvatar == "" || row.FollowedAbstract == "" {
			ids = append(ids, row.FollowedUserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	got, err := s.users.Displays(ctx, ids)
	if err != nil {
		return
	}
	for i := range rows {
		if user, ok := got[rows[i].FollowedUserID]; ok {
			if rows[i].FollowedNickname == "" {
				rows[i].FollowedNickname = user.Nickname
			}
			if rows[i].FollowedAvatar == "" {
				rows[i].FollowedAvatar = user.Avatar
			}
			if rows[i].FollowedAbstract == "" {
				rows[i].FollowedAbstract = user.Abstract
			}
		}
	}
}

// hydrateFans 补齐缺失的粉丝快照。
func (s *Service) hydrateFans(ctx context.Context, rows []domain.Follow) {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.FansNickname == "" || row.FansAvatar == "" || row.FansAbstract == "" {
			ids = append(ids, row.FansUserID)
		}
	}
	if len(ids) == 0 {
		return
	}
	got, err := s.users.Displays(ctx, ids)
	if err != nil {
		return
	}
	for i := range rows {
		if user, ok := got[rows[i].FansUserID]; ok {
			if rows[i].FansNickname == "" {
				rows[i].FansNickname = user.Nickname
			}
			if rows[i].FansAvatar == "" {
				rows[i].FansAvatar = user.Avatar
			}
			if rows[i].FansAbstract == "" {
				rows[i].FansAbstract = user.Abstract
			}
		}
	}
}

func (s *Service) logError(format string, args ...any) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}
