package redis_user

import (
	"context"
	"time"

	"myblogx/models/ctype"
	"myblogx/service/redis_service"
)

// TryMarkUserHomeViewed 使用 HSETNX 做“写入式判重”。
// 返回 true 表示今天第一次记录这次访问；返回 false 表示今天已经访问过。
func TryMarkUserHomeViewed(deps redis_service.Deps, userID, viewerUserID ctype.ID, now time.Time) (bool, error) {
	if deps.Client == nil {
		return true, nil
	}

	ctx := context.Background()
	key := statUserViewDailyKey(userID, now)
	field := viewerUserID.String()

	marked, err := deps.Client.HSetNX(ctx, key, field, 1).Result()
	if err != nil {
		return false, err
	}
	if marked {
		ttl := time.Until(nextDayStart(now))
		if ttl <= 0 {
			ttl = time.Second
		}
		if err = deps.Client.Expire(ctx, key, ttl).Err(); err != nil {
			_ = deps.Client.HDel(ctx, key, field).Err()
			return false, err
		}
	}
	return marked, nil
}

// RollbackUserHomeViewed 在数据库事务失败时尽力回滚 Redis 判重标记，
// 避免“Redis 已记过，但数据库未成功落库”导致当天少算 1 次。
func RollbackUserHomeViewed(deps redis_service.Deps, userID, viewerUserID ctype.ID, now time.Time) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.HDel(
		context.Background(),
		statUserViewDailyKey(userID, now),
		viewerUserID.String(),
	).Err()
}

func nextDayStart(now time.Time) time.Time {
	location := now.Location()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
}
