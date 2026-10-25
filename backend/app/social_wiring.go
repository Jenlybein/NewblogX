package app

import (
	"context"

	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/readmodel"
	socialhttp "myblogx/internal/social/adapter/http"
	socialmysql "myblogx/internal/social/adapter/mysql"
	socialapp "myblogx/internal/social/app"
	"myblogx/internal/social/domain"
	usermysql "myblogx/internal/user/adapter/mysql"

	"gorm.io/gorm"
)

// socialService 供 user/comment 模块读取关注关系。
var socialService *socialapp.Service

// WireSocial 装配社交模块并注入路由处理器。
func WireSocial(infra *Infra) {
	if infra == nil {
		return
	}
	svc := socialapp.New(
		socialmysql.New(infra.DB),
		&followStatsBridge{db: infra.DB},
		&displayBridge{db: infra.DB},
		&privacyBridge{db: infra.DB},
		infra.Logger,
	)
	socialService = svc
	router.SetFollowHandler(socialhttp.NewHandler(svc))
}

// followStatsBridge 更新 user_stat 的关注数/粉丝数。
//
// 旧实现在同一个事务里更新；这里退化成独立更新，失败只记日志。
// 这是已知债：S4.6 会把"更新统计"改成 outbox 事件，届时恢复最终一致。
type followStatsBridge struct {
	db *gorm.DB
}

// ApplyFollowDelta 实现 domain.UserStats。
func (b *followStatsBridge) ApplyFollowDelta(_ context.Context, fansID, followedID uint64, delta int) error {
	return usermysql.ApplyFollowDelta(b.db, uint64(fansID), uint64(followedID), delta)
}

// displayBridge 读用户展示快照。
type displayBridge struct {
	db *gorm.DB
}

// Displays 实现 domain.UserDisplay。
func (b *displayBridge) Displays(_ context.Context, ids []uint64) (map[uint64]domain.Display, error) {
	raw, err := readmodel.LoadUserDisplayMap(b.db, toCtypeIDs(ids))
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]domain.Display, len(raw))
	for id, item := range raw {
		out[uint64(id)] = domain.Display{Nickname: item.Nickname, Avatar: item.Avatar, Abstract: item.Abstract}
	}
	return out, nil
}

// confPrivacyRow 对应 user_conf_models 的可见性列。
type confPrivacyRow struct {
	UserID           uint64 `gorm:"primaryKey;autoIncrement:false"`
	FollowVisibility bool
	FansVisibility   bool
}

// TableName 固定表名。
func (confPrivacyRow) TableName() string { return "user_conf_models" }

// privacyBridge 读列表可见性。
type privacyBridge struct {
	db *gorm.DB
}

// FollowVisible 实现 domain.Privacy。
func (b *privacyBridge) FollowVisible(ctx context.Context, userID uint64) (bool, error) {
	row, err := b.load(ctx, userID)
	if err != nil {
		return false, err
	}
	return row.FollowVisibility, nil
}

// FansVisible 实现 domain.Privacy。
func (b *privacyBridge) FansVisible(ctx context.Context, userID uint64) (bool, error) {
	row, err := b.load(ctx, userID)
	if err != nil {
		return false, err
	}
	return row.FansVisibility, nil
}

func (b *privacyBridge) load(ctx context.Context, userID uint64) (confPrivacyRow, error) {
	var row confPrivacyRow
	err := b.db.WithContext(ctx).Select("user_id", "follow_visibility", "fans_visibility").
		Take(&row, "user_id = ?", userID).Error
	return row, err
}
