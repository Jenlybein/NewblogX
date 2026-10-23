package user_service

import (
	"fmt"
	"myblogx/models"
	"myblogx/models/ctype"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StatEnsureRows 确保用户统计行存在
// 作用：自动为用户创建统计记录（关注数、粉丝数等）
// 适用场景：新用户注册自动初始化、老用户历史数据回填
// 参数：tx 数据库事务，userIDs 要初始化统计的用户ID列表
func StatEnsureRows(tx *gorm.DB, userIDs ...ctype.ID) error {
	// 空值校验：无事务或无用户ID时直接返回
	if tx == nil || len(userIDs) == 0 {
		return nil
	}

	// 初始化用户统计模型切片，预分配容量提升性能
	rows := make([]models.UserStatModel, 0, len(userIDs))
	// 去重map：避免重复插入同一个用户的统计记录
	seen := make(map[ctype.ID]struct{}, len(userIDs))

	// 遍历用户ID，过滤无效值并去重
	for _, userID := range userIDs {
		// 跳过空ID
		if userID.IsZero() {
			continue
		}
		// 跳过已存在的ID（去重）
		if _, ok := seen[userID]; ok {
			continue
		}
		// 标记为已处理
		seen[userID] = struct{}{}
		// 构建统计记录（仅需UserID，其余字段使用数据库默认值）
		rows = append(rows, models.UserStatModel{UserID: userID})
	}

	// 无有效用户ID时直接返回
	if len(rows) == 0 {
		return nil
	}

	// 批量插入用户统计记录
	// OnConflict{DoNothing: true}：主键冲突时不报错（已存在则跳过）
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// StatApplyFollowDelta 批量更新关注/粉丝统计数（单条SQL完成双向更新）
// 作用：关注/取关时，同时修改【我的关注数】和【对方的粉丝数】
// 参数：tx 数据库事务，fansUserID 粉丝（操作者）ID，followedUserID 被关注者ID，delta 变动值（+1关注/-1取关）
func StatApplyFollowDelta(tx *gorm.DB, fansUserID, followedUserID ctype.ID, delta int) error {
	// 事务不能为空校验
	if tx == nil {
		return fmt.Errorf("数据库事务不能为空")
	}

	// 先确保两个用户的统计记录都存在
	if err := StatEnsureRows(tx, fansUserID, followedUserID); err != nil {
		return err
	}

	// 定义基础更新表达式：关注数 + delta / 粉丝数 + delta
	followExpr := "CASE WHEN user_id = ? THEN follow_count + ? ELSE follow_count END"
	fansExpr := "CASE WHEN user_id = ? THEN fans_count + ? ELSE fans_count END"

	// 如果是取关操作（delta为负数），增加数值下限保护：不允许计数 < 0
	if delta < 0 {
		// 关注数：计算后小于0则置为0，否则正常累加
		followExpr = "CASE WHEN user_id = ? THEN CASE WHEN follow_count + ? < 0 THEN 0 ELSE follow_count + ? END ELSE follow_count END"
		// 粉丝数：同上，防止负数
		fansExpr = "CASE WHEN user_id = ? THEN CASE WHEN fans_count + ? < 0 THEN 0 ELSE fans_count + ? END ELSE fans_count END"
	}

	// 构造更新字段映射
	updates := map[string]any{}
	if delta > 0 {
		// 关注操作：使用基础表达式（2个占位符）
		updates["follow_count"] = gorm.Expr(followExpr, fansUserID, delta)
		updates["fans_count"] = gorm.Expr(fansExpr, followedUserID, delta)
	} else {
		// 取关操作：使用防负数表达式（3个占位符）
		updates["follow_count"] = gorm.Expr(followExpr, fansUserID, delta, delta)
		updates["fans_count"] = gorm.Expr(fansExpr, followedUserID, delta, delta)
	}

	// 执行更新：只更新指定的两个用户统计记录
	result := tx.Model(&models.UserStatModel{}).
		Where("user_id IN ?", []ctype.ID{fansUserID, followedUserID}).
		Updates(updates)

	// 更新执行失败
	if result.Error != nil {
		return result.Error
	}

	// 完整性校验：必须同时更新2条记录（我的关注数、对方粉丝数）
	if result.RowsAffected < 2 {
		return fmt.Errorf("用户统计更新不完整: fans_user_id=%d followed_user_id=%d rows=%d", fansUserID, followedUserID, result.RowsAffected)
	}

	return nil
}
