package app

import (
	"context"

	articlemysql "myblogx/internal/article/adapter/mysql"
	"myblogx/internal/kernel/ctype"
	userapp "myblogx/internal/user/app"
	userdomain "myblogx/internal/user/domain"

	"gorm.io/gorm"
)

// authorStatsBridge 把 article 模块的汇总能力接成 user 模块的 AuthorStatsReader。
//
// 汇总口径（只算已发布文章）在 article 模块的适配器里，这里只做 ID 类型转换；
// 两个模块之间依旧没有 import 关系——桥接只出现在装配层。
type authorStatsBridge struct {
	db *gorm.DB
}

// AuthorStats 实现 userapp.AuthorStatsReader。
func (b *authorStatsBridge) AuthorStats(ctx context.Context, id userdomain.UserID) (userapp.AuthorStats, error) {
	raw, err := articlemysql.AuthorStats(ctx, b.db, ctype.NewID(id.Uint64()))
	if err != nil {
		return userapp.AuthorStats{}, err
	}
	return userapp.AuthorStats{
		Digg:    raw.Digg,
		Favor:   raw.Favor,
		Comment: raw.Comment,
		Visited: raw.Visited,
	}, nil
}
