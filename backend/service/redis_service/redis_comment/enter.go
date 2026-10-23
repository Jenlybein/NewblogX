package redis_comment

import (
	"context"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"strconv"
)

const ReplyCountCacheKey = "comment_reply"
const DiggCountCacheKey = "comment_digg"

type BatchCounters struct {
	ReplyMap map[ctype.ID]int
	DiggMap  map[ctype.ID]int
}

func SetCacheReply(deps redis_service.Deps, commentID ctype.ID, increase int) error {
	return set(deps, ReplyCountCacheKey, commentID, increase)
}

func GetCacheReply(deps redis_service.Deps, commentID ctype.ID) int {
	return get(deps, ReplyCountCacheKey, commentID)
}

func DelCacheReply(deps redis_service.Deps, commentID ctype.ID) error {
	return del(deps, ReplyCountCacheKey, commentID)
}

func GetBatchCacheReply(deps redis_service.Deps, commentIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, ReplyCountCacheKey, commentIDs)
}

func GetAllCacheReply(deps redis_service.Deps) map[ctype.ID]int {
	return getAll(deps, ReplyCountCacheKey)
}

func ClearAllCacheReply(deps redis_service.Deps) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.Del(context.Background(), ReplyCountCacheKey).Err()
}

func SetCacheDigg(deps redis_service.Deps, commentID ctype.ID, increase int) error {
	return set(deps, DiggCountCacheKey, commentID, increase)
}

func GetCacheDigg(deps redis_service.Deps, commentID ctype.ID) int {
	return get(deps, DiggCountCacheKey, commentID)
}

func DelCacheDigg(deps redis_service.Deps, commentID ctype.ID) error {
	return del(deps, DiggCountCacheKey, commentID)
}

func GetBatchCacheDigg(deps redis_service.Deps, commentIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, DiggCountCacheKey, commentIDs)
}

func GetBatchCounters(deps redis_service.Deps, commentIDs []ctype.ID) BatchCounters {
	counters := BatchCounters{
		ReplyMap: make(map[ctype.ID]int),
		DiggMap:  make(map[ctype.ID]int),
	}
	if deps.Client == nil || len(commentIDs) == 0 {
		return counters
	}

	fields := make([]string, 0, len(commentIDs))
	for _, commentID := range commentIDs {
		fields = append(fields, commentID.String())
	}

	ctx := context.Background()
	pipe := deps.Client.Pipeline()
	defer pipe.Close()

	replyCmd := pipe.HMGet(ctx, ReplyCountCacheKey, fields...)
	diggCmd := pipe.HMGet(ctx, DiggCountCacheKey, fields...)

	if _, err := pipe.Exec(ctx); err != nil {
		return counters
	}

	if values, err := replyCmd.Result(); err == nil {
		counters.ReplyMap = decodeBatchValues(commentIDs, values)
	}
	if values, err := diggCmd.Result(); err == nil {
		counters.DiggMap = decodeBatchValues(commentIDs, values)
	}
	return counters
}

func GetAllCacheDigg(deps redis_service.Deps) map[ctype.ID]int {
	return getAll(deps, DiggCountCacheKey)
}

func ClearAllCacheDigg(deps redis_service.Deps) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.Del(context.Background(), DiggCountCacheKey).Err()
}

func set(deps redis_service.Deps, key string, commentID ctype.ID, increase int) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.HIncrBy(context.Background(), key, commentID.String(), int64(increase)).Err()
}

func get(deps redis_service.Deps, key string, commentID ctype.ID) int {
	if deps.Client == nil {
		return 0
	}
	num, _ := deps.Client.HGet(context.Background(), key, commentID.String()).Int()
	return num
}

func del(deps redis_service.Deps, key string, commentID ctype.ID) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.HDel(context.Background(), key, commentID.String()).Err()
}

func getBatch(deps redis_service.Deps, key string, commentIDs []ctype.ID) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(commentIDs))
	if deps.Client == nil || len(commentIDs) == 0 {
		return result
	}

	fields := make([]string, 0, len(commentIDs))
	for _, commentID := range commentIDs {
		fields = append(fields, commentID.String())
	}

	values, err := deps.Client.HMGet(context.Background(), key, fields...).Result()
	if err != nil {
		return result
	}

	return decodeBatchValues(commentIDs, values)
}

func getAll(deps redis_service.Deps, key string) map[ctype.ID]int {
	if deps.Client == nil {
		return nil
	}
	res, err := deps.Client.HGetAll(context.Background(), key).Result()
	if err != nil {
		return nil
	}

	numMap := make(map[ctype.ID]int, len(res))
	for k, v := range res {
		commentID, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			continue
		}
		num, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		numMap[ctype.ID(commentID)] = num
	}
	return numMap
}

func decodeBatchValues(commentIDs []ctype.ID, values []any) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(commentIDs))
	for i, raw := range values {
		if raw == nil || i >= len(commentIDs) {
			continue
		}
		num, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil {
			continue
		}
		result[commentIDs[i]] = num
	}
	return result
}
