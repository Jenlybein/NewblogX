package mysql

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"myblogx/internal/kernel/ctype"
)

// 本文件承载「跨模块写用户统计」的能力。
//
// 它原本在 service/user_service，被 article 与 social 的装配直接调用。
// 迁到本包之后，跨模块调用走端口（article 的 AuthorStatsPort 由装配层注入函数），
// 业务模块不再 import 另一个模块的 service 包。
//
// 这些函数都接受 tx *gorm.DB 而不是自己开事务：调用方要在自己的事务里
// 完成"删文章 + 作者文章数 -1"，拆成两个事务会让用户看到中间状态。

// ApplyArticleDelta 更新作者的文章数与文章累计浏览数。
//
// 负数走 CASE 保护到 0：删文章时如果作者计数已经不一致，不该变成负数
// （负的"文章数"在页面上比少 1 更难解释）。
func ApplyArticleDelta(tx *gorm.DB, authorID uint64, articleDelta, viewDelta int) error {
	if tx == nil {
		return fmt.Errorf("stat: db 不能为空")
	}
	if authorID == 0 || (articleDelta == 0 && viewDelta == 0) {
		return nil
	}

	if err := EnsureStatRows(tx, authorID); err != nil {
		return err
	}

	updates := map[string]any{}
	if articleDelta != 0 {
		updates["article_count"] = deltaExpr("article_count", articleDelta)
	}
	if viewDelta != 0 {
		updates["article_visited_count"] = deltaExpr("article_visited_count", viewDelta)
	}

	result := tx.Model(&UserStatRow{}).Where("user_id = ?", authorID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("stat: 作者文章统计更新未命中 author_id=%d", authorID)
	}
	return nil
}

// deltaExpr 生成增减表达式；负数带下限 0 保护。
func deltaExpr(column string, delta int) any {
	if delta > 0 {
		return gorm.Expr(column+" + ?", delta)
	}
	return gorm.Expr(
		"CASE WHEN "+column+" + ? < 0 THEN 0 ELSE "+column+" + ? END",
		delta, delta,
	)
}

// EnsureStatRows 确保统计行存在（注册时初始化，历史数据回填时也用它）。
func EnsureStatRows(tx *gorm.DB, userIDs ...uint64) error {
	if tx == nil || len(userIDs) == 0 {
		return nil
	}

	rows := make([]UserStatRow, 0, len(userIDs))
	seen := make(map[uint64]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		rows = append(rows, UserStatRow{UserID: ctype.NewID(id)})
	}
	if len(rows) == 0 {
		return nil
	}

	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// ApplyFollowDelta 一次 SQL 同时更新「我的关注数」与「对方粉丝数」。
//
// 用 CASE 而不是两条 UPDATE：两条语句之间会有短暂状态不一致，
// 而且任一失败就没法回滚到"两边都没变"（调用方虽有事务，但少一次往返更省）。
func ApplyFollowDelta(tx *gorm.DB, fansUserID, followedUserID uint64, delta int) error {
	if tx == nil {
		return fmt.Errorf("stat: db 不能为空")
	}
	if err := EnsureStatRows(tx, fansUserID, followedUserID); err != nil {
		return err
	}

	followExpr := "CASE WHEN user_id = ? THEN follow_count + ? ELSE follow_count END"
	fansExpr := "CASE WHEN user_id = ? THEN fans_count + ? ELSE fans_count END"
	if delta < 0 {
		followExpr = "CASE WHEN user_id = ? THEN CASE WHEN follow_count + ? < 0 THEN 0 ELSE follow_count + ? END ELSE follow_count END"
		fansExpr = "CASE WHEN user_id = ? THEN CASE WHEN fans_count + ? < 0 THEN 0 ELSE fans_count + ? END ELSE fans_count END"
	}

	updates := map[string]any{}
	if delta > 0 {
		updates["follow_count"] = gorm.Expr(followExpr, fansUserID, delta)
		updates["fans_count"] = gorm.Expr(fansExpr, followedUserID, delta)
	} else {
		updates["follow_count"] = gorm.Expr(followExpr, fansUserID, delta, delta)
		updates["fans_count"] = gorm.Expr(fansExpr, followedUserID, delta, delta)
	}

	result := tx.Model(&UserStatRow{}).
		Where("user_id IN ?", []uint64{fansUserID, followedUserID}).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected < 2 {
		return fmt.Errorf("stat: 关注统计更新不完整 fans=%d followed=%d rows=%d",
			fansUserID, followedUserID, result.RowsAffected)
	}
	return nil
}

// RecordViewDaily 记录一次"某访客浏览某主页"，返回本次是否真正计入。
//
// 靠唯一索引去重：插入成功才算 +1，冲突（同一访客当天已看过）返回 false。
// **必须是硬性的一次性写入 + 计数**：放在调用方的事务里，避免"日表插了、
// 计数没加"这种只有一边成功的状态。
func RecordViewDaily(tx *gorm.DB, targetID, viewerID uint64, viewDate time.Time) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("stat: db 不能为空")
	}
	if targetID == 0 || viewerID == 0 || targetID == viewerID {
		return false, nil
	}

	row := UserViewDailyRow{
		UserID:       ctype.NewID(targetID),
		ViewerUserID: ctype.NewID(viewerID),
		ViewDate:     viewDate,
	}
	created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if created.Error != nil {
		return false, created.Error
	}
	if created.RowsAffected == 0 {
		// 当天已记过，不重复计数。
		return false, nil
	}

	if err := EnsureStatRows(tx, targetID); err != nil {
		return false, err
	}
	updated := tx.Model(&UserStatRow{}).
		Where("user_id = ?", targetID).
		Update("view_count", gorm.Expr("view_count + 1"))
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected == 0 {
		return false, fmt.Errorf("stat: 浏览统计更新未命中 user_id=%d", targetID)
	}
	return true, nil
}
