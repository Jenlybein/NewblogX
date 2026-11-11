package ddl

import "gorm.io/gorm"

// BackfillFavoriteCounts 重算收藏夹的冗余计数列（favorite_models.article_count）。
//
// 为什么需要它：`article_count` 从重构起就**没有任何写入**——新建的夹子是 0，
// 历史值也全部是 0 或过期值，列表上的「N 篇」于是长期说谎（#344）。写入侧
// 现在会在成员变更的同一事务内刷新该列，但历史数据必须补一次。
//
// 幂等：整列按关系行重算，与当前值无关；可重复执行（部署时随 `-db` 跑一次即可）。
// 只写这一列，不做任何结构变更 —— 零 DDL 口径不受影响。
func BackfillFavoriteCounts(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.Exec(`
		UPDATE favorite_models AS f
		SET article_count = (
			SELECT COUNT(*) FROM user_article_favor_models r
			WHERE r.favor_id = f.id AND r.deleted_at IS NULL
		)
		WHERE f.deleted_at IS NULL`).Error
}
