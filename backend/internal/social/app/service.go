// Package app 是社交（关注关系）模块的用例层。
package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
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
		FollowedUserID:   ctype.NewID(targetID),
		FollowedNickname: followed.Nickname,
		FollowedAvatar:   followed.Avatar,
		FollowedAbstract: followed.Abstract,
		FansUserID:       ctype.NewID(viewerID),
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

// ListRelations 关系列表（关注 / 粉丝 / 互关共用同一个入口）。
func (s *Service) ListRelations(ctx context.Context, viewerID, ownerID uint64, kind domain.RelationKind, page domain.Page) ([]domain.RelationItem, bool, error) {
	ownerID = s.resolveOwner(viewerID, ownerID)
	if ownerID == 0 {
		return []domain.RelationItem{}, false, nil
	}
	if ownerID != viewerID {
		if err := s.ensureVisible(ctx, ownerID, kind); err != nil {
			return nil, false, err
		}
	}

	rows, hasMore, err := s.repo.ListRelations(ctx, ownerID, kind, page)
	if err != nil {
		return nil, false, err
	}
	return s.toRelationItems(ctx, viewerID, kind, rows), hasMore, nil
}

// CountRelations 三个视图的计数（顶栏徽标用）。
func (s *Service) CountRelations(ctx context.Context, viewerID, ownerID uint64) (domain.RelationCounts, error) {
	ownerID = s.resolveOwner(viewerID, ownerID)
	if ownerID == 0 {
		return domain.RelationCounts{}, nil
	}
	// 计数跟列表一起受可见性约束：列表锁着而数字还亮着，等于把私密关系的
	// 规模透给访客。
	if ownerID != viewerID {
		if err := s.ensureVisible(ctx, ownerID, domain.KindFollow); err != nil {
			return domain.RelationCounts{}, err
		}
	}
	return s.repo.CountRelations(ctx, ownerID)
}

// RemoveFan 移除一个关注我的人。
//
// 语义是**单向移除**：只删"他关注我"这一条，不通知对方，也不阻止他再次关注。
// 所以它调用的持久化操作与取关完全相同（Unfollow(我, 他)），差别只在用例层：
// 取关是我放弃了对他的关注，移除粉丝是我让别人对我的关注消失。
//
// 统计照旧跟着关系走：他的关注数 -1、我的粉丝数 -1。
func (s *Service) RemoveFan(ctx context.Context, viewerID, fanID uint64) error {
	if viewerID == 0 {
		return apperr.New(apperr.CodeAuthRequired)
	}
	if viewerID == fanID {
		return apperr.New(apperr.CodeFollowSelf)
	}

	removed, err := s.repo.Unfollow(ctx, viewerID, fanID)
	if err != nil {
		return err
	}
	if !removed {
		return apperr.New(apperr.CodeFollowFanNotFound)
	}
	if s.stats != nil {
		if err := s.stats.ApplyFollowDelta(ctx, fanID, viewerID, -1); err != nil {
			s.logError("更新移除粉丝后的统计失败: fans=%d followed=%d err=%v", fanID, viewerID, err)
		}
	}
	return nil
}

// ensureVisible 检查 owner 的对应列表是否对访客可见。
//
// mutual ⊆ follow，所以互关视图沿用关注列表的可见性开关。
func (s *Service) ensureVisible(ctx context.Context, ownerID uint64, kind domain.RelationKind) error {
	if s.privacy == nil {
		return nil
	}
	var (
		visible bool
		err     error
	)
	if kind == domain.KindFans {
		visible, err = s.privacy.FansVisible(ctx, ownerID)
	} else {
		visible, err = s.privacy.FollowVisible(ctx, ownerID)
	}
	if err != nil {
		return apperr.New(apperr.CodeFollowConfigMissing)
	}
	if !visible {
		return apperr.New(apperr.CodeFollowListPrivate)
	}
	return nil
}

// toRelationItems 把关系行投影成统一条目，并补齐关系值。
//
// 关系值是"我看这些人分别是什么关系"，所以要用 viewer 去算；缺快照的那一端
// 顺手用展示读补上（历史数据里快照可能为空）。
func (s *Service) toRelationItems(ctx context.Context, viewerID uint64, kind domain.RelationKind, rows []domain.Follow) []domain.RelationItem {
	if kind == domain.KindFans {
		s.hydrateFans(ctx, rows)
	} else {
		s.hydrateFollowed(ctx, rows)
	}

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		id, _, _, _ := row.Counterpart(kind)
		ids = append(ids, id.Uint64())
	}
	relations := s.relationBatch(ctx, viewerID, ids)

	items := make([]domain.RelationItem, 0, len(rows))
	for _, row := range rows {
		id, nickname, avatar, abstract := row.Counterpart(kind)
		items = append(items, domain.RelationItem{
			UserID:     id,
			Nickname:   nickname,
			Avatar:     avatar,
			Abstract:   abstract,
			FollowTime: row.CreatedAt,
			Relation:   relations[id.Uint64()],
		})
	}
	return items
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
			ids = append(ids, row.FollowedUserID.Uint64())
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
		if user, ok := got[rows[i].FollowedUserID.Uint64()]; ok {
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
			ids = append(ids, row.FansUserID.Uint64())
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
		if user, ok := got[rows[i].FansUserID.Uint64()]; ok {
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
