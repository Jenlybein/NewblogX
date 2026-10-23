package redis_tag

import (
	"context"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"strconv"
)

const TagCacheArticleCount = "tag_article_count"

func SetCacheArticleCount(deps redis_service.Deps, tagID ctype.ID, increase int) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.HIncrBy(context.Background(), TagCacheArticleCount, tagID.String(), int64(increase)).Err()
}

func GetCacheArticleCount(deps redis_service.Deps, tagID ctype.ID) int {
	if deps.Client == nil {
		return 0
	}
	num, _ := deps.Client.HGet(context.Background(), TagCacheArticleCount, tagID.String()).Int()
	return num
}

func GetBatchCacheArticleCount(deps redis_service.Deps, tagIDs []ctype.ID) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(tagIDs))
	if len(tagIDs) == 0 {
		return result
	}
	if deps.Client == nil {
		return result
	}

	fields := make([]string, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		fields = append(fields, tagID.String())
	}

	values, err := deps.Client.HMGet(context.Background(), TagCacheArticleCount, fields...).Result()
	if err != nil {
		return result
	}

	for i, raw := range values {
		if raw == nil {
			continue
		}
		num, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil {
			continue
		}
		result[tagIDs[i]] = num
	}
	return result
}

func GetAllCacheArticleCount(deps redis_service.Deps) map[ctype.ID]int {
	if deps.Client == nil {
		return map[ctype.ID]int{}
	}
	res, err := deps.Client.HGetAll(context.Background(), TagCacheArticleCount).Result()
	if err != nil {
		return nil
	}

	numMap := make(map[ctype.ID]int, len(res))
	for k, v := range res {
		tagID, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		num, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		numMap[ctype.ID(tagID)] = num
	}
	return numMap
}

func ClearAllCacheTag(deps redis_service.Deps) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.Del(context.Background(), TagCacheArticleCount).Err()
}
