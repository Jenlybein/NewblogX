package mysql

import (
	"gorm.io/gorm"
)

// 本文件承担「把 Redis 增量计数并账回文章/标签表」的写入。
//
// 这些 SQL 原本在 service/cron_service 里，用 `s.db.Model(&models.ArticleModel{})`（旧包，已删）
// 直接写。迁到本包后：表结构由本包的 row 定义，列名受类型约束，
// cron 侧只负责"取增量、调这里、失败回补"，不再持有任何 SQL。

// ArticleCounter 是被并账的文章计数列。
//
// 用类型而不是裸字符串：列名会被拼进 UPDATE 表达式，任何"来自变量"的列名
// 都是注入面。这里是编译期常量集合，调用方传不进任意值。
type ArticleCounter string

// 文章计数的四个列。
const (
	ArticleFavorCount   ArticleCounter = "favor_count"
	ArticleDiggCount    ArticleCounter = "digg_count"
	ArticleViewCount    ArticleCounter = "view_count"
	ArticleCommentCount ArticleCounter = "comment_count"
)

// relationRowOf 返回"这个计数列的权威来源是哪张关系表的行"。
//
// 只有两个社列有关系行来源（ADR-0025 决策 1）；返回 nil 表示这一列没有关系行，
// 只能继续按增量累加：
//   - `view_count`：浏览量是"匿名也记一次"的事件，没有关系行可数；
//   - `comment_count`：由评论模块自己维护（删评论、审核状态不止"行数"），
//     本批明确不动它的语义。
func relationRowOf(col ArticleCounter) any {
	switch col {
	case ArticleDiggCount:
		return &articleDiggRow{}
	case ArticleFavorCount:
		return &userArticleFavorRow{}
	}
	return nil
}

// ReconcileArticleCount 并账一篇文章的一个计数列，返回 hit=false 表示文章不存在
// （已被删除）。这种情况**不返回 error**：调用方若把它当失败就会把增量回补到 Redis，
// 下一轮再撞同一条、无限重试。正确处置是丢弃这条增量并记一条 Warn。
//
// 两种写库方式（ADR-0025）：
//   - 有关系行来源的列（digg / favor）：用 `COUNT(关系行)` **重算覆盖**整列，
//     delta 完全不参与运算。历史漂移因此在本轮自愈，不需要额外对账任务；
//   - 没有关系行来源的列（view / comment）：照旧 `列 += delta`。
//
// # 并账瞬间的临时 ±1 为什么不累积
//
// 刷盘顺序是「先把 active key 原子 RENAME 成 sync key（此刻这批增量被定格），
// 再逐条 COUNT 并写入」。于是同一次事件有可能被算两遍：关系行先落库、
// HINCRBY 晚于 RENAME 时，它既进了本轮的 COUNT，又进了下一轮的新 active key
// ——读出来就是临时 +1。反过来（增量已被定格、关系事务尚未提交）是本轮少算 1。
// 两种情况都**不累积**：这里每次都是拿 COUNT 覆盖整列，不做 `列 += 增量`，
// 所以该文章下一次进入增量批次时会被覆盖回关系行的真值（漂移只可能活到那一次，
// 覆盖面与 ADR-0025 认下的「只对这批有增量的 id 做」一致）。累加式才会把一次
// 丢失永久留在账上，这正是本 ADR 要根除的形态。
func ReconcileArticleCount(db *gorm.DB, col ArticleCounter, articleID uint64, delta int) (hit bool, err error) {
	if db == nil {
		return false, nil
	}
	if relation := relationRowOf(col); relation != nil {
		return recomputeArticleCount(db, col, relation, articleID)
	}
	return applyArticleDelta(db, col, articleID, delta)
}

// recomputeArticleCount 用关系行条数覆盖计数列。
//
// 先判文章是否还在，而不是靠 UPDATE 的 RowsAffected：MySQL 的 affected rows 是
// "真正被改动的行数"，新值与旧值相同时返回 0，会把它误判成"文章不存在"并打出
// 一条假的"丢弃增量"。软删的文章按不存在处理（与 UPDATE 的软删范围一致）。
func recomputeArticleCount(db *gorm.DB, col ArticleCounter, relation any, articleID uint64) (bool, error) {
	var live int64
	if err := db.Model(&articleRow{}).Where("id = ?", articleID).Count(&live).Error; err != nil {
		return false, err
	}
	if live == 0 {
		return false, nil
	}

	// Model(relation) 会带上 `deleted_at IS NULL`：取消点赞 / 移出收藏夹都是软删，
	// 残行仍占着唯一键，但不再是"一条关系"。
	var rows int64
	if err := db.Model(relation).Where("article_id = ?", articleID).Count(&rows).Error; err != nil {
		return false, err
	}
	if err := db.Model(&articleRow{}).Where("id = ?", articleID).
		UpdateColumn(string(col), int(rows)).Error; err != nil {
		return false, err
	}
	return true, nil
}

// applyArticleDelta 按增量更新没有关系行来源的计数列。
//
// 负数带 0 下限：取消点赞 / 收藏过多时不该出现负计数，那在页面上比少 1 更难解释。
func applyArticleDelta(db *gorm.DB, col ArticleCounter, articleID uint64, delta int) (hit bool, err error) {
	expr := "CASE WHEN " + string(col) + " + ? < 0 THEN 0 ELSE " + string(col) + " + ? END"
	res := db.Model(&articleRow{}).
		Where("id = ?", articleID).
		UpdateColumn(string(col), gorm.Expr(expr, delta, delta))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// FlushTagArticleCount 按增量更新标签的文章数。
//
// 标签文章数没有等价关系行（多对多的 join 表随标签/文章任一方变化，
// 口径不是"行数"），因此保持累加；hit=false 的处置同 ReconcileArticleCount
// （标签被删则丢弃增量）。
func FlushTagArticleCount(db *gorm.DB, tagID uint64, delta int) (hit bool, err error) {
	if db == nil {
		return false, nil
	}
	const expr = "CASE WHEN article_count + ? < 0 THEN 0 ELSE article_count + ? END"
	res := db.Model(&tagRow{}).
		Where("id = ?", tagID).
		UpdateColumn("article_count", gorm.Expr(expr, delta, delta))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
