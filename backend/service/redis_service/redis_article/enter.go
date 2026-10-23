package redis_article

import (
	"context"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"strconv"
	"time"
)

type ArticleCacheType string

// BatchCounters 汇总文章在 Redis 中的实时计数增量。
// 这些值会叠加到数据库或 ES 中的基础值上，用于列表实时展示。
type BatchCounters struct {
	ViewMap    map[ctype.ID]int
	DiggMap    map[ctype.ID]int
	FavorMap   map[ctype.ID]int
	CommentMap map[ctype.ID]int
}

// 文章缓存的Key
const (
	ArticleCacheView     ArticleCacheType = "article_view"
	ArticleCacheDigg     ArticleCacheType = "article_digg"
	ArticleCacheFavorite ArticleCacheType = "article_favorite"
	ArticleCacheComment  ArticleCacheType = "article_comment"
)

// 设置缓存
func set(deps redis_service.Deps, t ArticleCacheType, articleID ctype.ID, increase int) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.HIncrBy(context.Background(), string(t), articleID.String(), int64(increase)).Err()
}

func get(deps redis_service.Deps, t ArticleCacheType, articleID ctype.ID) int {
	if deps.Client == nil {
		return 0
	}
	num, _ := deps.Client.HGet(context.Background(), string(t), articleID.String()).Int()
	return num
}

// 浏览量缓存
func SetCacheView(deps redis_service.Deps, articleID ctype.ID, increase int) error {
	return set(deps, ArticleCacheView, articleID, increase)
}
func GetCacheView(deps redis_service.Deps, articleID ctype.ID) int {
	return get(deps, ArticleCacheView, articleID)
}

// 点赞缓存
func SetCacheDigg(deps redis_service.Deps, articleID ctype.ID, increase int) error {
	return set(deps, ArticleCacheDigg, articleID, increase)
}
func GetCacheDigg(deps redis_service.Deps, articleID ctype.ID) int {
	return get(deps, ArticleCacheDigg, articleID)
}

// 收藏缓存
func SetCacheFavorite(deps redis_service.Deps, articleID ctype.ID, increase int) error {
	return set(deps, ArticleCacheFavorite, articleID, increase)
}
func GetCacheFavorite(deps redis_service.Deps, articleID ctype.ID) int {
	return get(deps, ArticleCacheFavorite, articleID)
}

// 评论缓存
func SetCacheComment(deps redis_service.Deps, articleID ctype.ID, increase int) error {
	return set(deps, ArticleCacheComment, articleID, increase)
}
func GetCacheComment(deps redis_service.Deps, articleID ctype.ID) int {
	return get(deps, ArticleCacheComment, articleID)
}

func GetAll(deps redis_service.Deps, t ArticleCacheType) map[ctype.ID]int {
	if deps.Client == nil {
		return nil
	}
	res, err := deps.Client.HGetAll(context.Background(), string(t)).Result()
	if err != nil {
		return nil
	}
	numMap := make(map[ctype.ID]int)
	for k, v := range res {
		ik, err := strconv.Atoi(k)
		num, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		numMap[ctype.ID(ik)] = num
	}

	return numMap
}

func getBatch(deps redis_service.Deps, t ArticleCacheType, articleIDs []ctype.ID) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(articleIDs))
	if deps.Client == nil || len(articleIDs) == 0 {
		return result
	}

	values, err := deps.Client.HMGet(context.Background(), string(t), buildBatchFields(articleIDs)...).Result()
	if err != nil {
		return result
	}
	return decodeBatchValues(articleIDs, values)
}

func GetBatchCacheView(deps redis_service.Deps, articleIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, ArticleCacheView, articleIDs)
}
func GetBatchCacheDigg(deps redis_service.Deps, articleIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, ArticleCacheDigg, articleIDs)
}
func GetBatchCacheFavorite(deps redis_service.Deps, articleIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, ArticleCacheFavorite, articleIDs)
}
func GetBatchCacheComment(deps redis_service.Deps, articleIDs []ctype.ID) map[ctype.ID]int {
	return getBatch(deps, ArticleCacheComment, articleIDs)
}

// GetBatchCounters 通过一次 Redis Pipeline 批量读取文章的四类计数增量，
// 减少搜索列表阶段的 Redis 往返次数。
func GetBatchCounters(deps redis_service.Deps, articleIDs []ctype.ID) BatchCounters {
	counters := BatchCounters{
		ViewMap:    make(map[ctype.ID]int),
		DiggMap:    make(map[ctype.ID]int),
		FavorMap:   make(map[ctype.ID]int),
		CommentMap: make(map[ctype.ID]int),
	}
	if deps.Client == nil || len(articleIDs) == 0 {
		return counters
	}

	ctx := context.Background()
	fields := buildBatchFields(articleIDs)
	pipe := deps.Client.Pipeline()
	defer pipe.Close()

	viewCmd := pipe.HMGet(ctx, string(ArticleCacheView), fields...)
	diggCmd := pipe.HMGet(ctx, string(ArticleCacheDigg), fields...)
	favorCmd := pipe.HMGet(ctx, string(ArticleCacheFavorite), fields...)
	commentCmd := pipe.HMGet(ctx, string(ArticleCacheComment), fields...)

	if _, err := pipe.Exec(ctx); err != nil {
		return counters
	}

	if values, err := viewCmd.Result(); err == nil {
		counters.ViewMap = decodeBatchValues(articleIDs, values)
	}
	if values, err := diggCmd.Result(); err == nil {
		counters.DiggMap = decodeBatchValues(articleIDs, values)
	}
	if values, err := favorCmd.Result(); err == nil {
		counters.FavorMap = decodeBatchValues(articleIDs, values)
	}
	if values, err := commentCmd.Result(); err == nil {
		counters.CommentMap = decodeBatchValues(articleIDs, values)
	}

	return counters
}

func buildBatchFields(articleIDs []ctype.ID) []string {
	fields := make([]string, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		fields = append(fields, articleID.String())
	}
	return fields
}

func decodeBatchValues(articleIDs []ctype.ID, values []any) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(articleIDs))
	for i, raw := range values {
		if raw == nil || i >= len(articleIDs) {
			continue
		}
		num, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil {
			continue
		}
		result[articleIDs[i]] = num
	}
	return result
}

func GetAllCacheView(deps redis_service.Deps) map[ctype.ID]int {
	return GetAll(deps, ArticleCacheView)
}
func GetAllCacheDigg(deps redis_service.Deps) map[ctype.ID]int {
	return GetAll(deps, ArticleCacheDigg)
}
func GetAllCacheFavorite(deps redis_service.Deps) map[ctype.ID]int {
	return GetAll(deps, ArticleCacheFavorite)
}
func GetAllCacheComment(deps redis_service.Deps) map[ctype.ID]int {
	return GetAll(deps, ArticleCacheComment)
}

func ClearAllCacheArticle(deps redis_service.Deps) error {
	if deps.Client == nil {
		return nil
	}
	return deps.Client.Del(
		context.Background(),
		string(ArticleCacheView),
		string(ArticleCacheDigg),
		string(ArticleCacheFavorite),
		string(ArticleCacheComment),
	).Err()
}

// 设置用户阅读历史
func SetUserArticleHistoryCache(deps redis_service.Deps, articleID, userID int) {
	if deps.Client == nil {
		return
	}
	key := fmt.Sprintf("user_history_%d", userID)
	field := fmt.Sprintf("%d", articleID)

	now := time.Now()
	nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())

	if err := deps.Client.HSet(context.Background(), key, field, "").Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("写入用户阅读历史缓存失败: 错误=%v", err)
		}
		return
	}

	if err := deps.Client.ExpireAt(context.Background(), key, nextDay).Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("设置用户阅读历史缓存过期时间失败: 错误=%v", err)
		}
		return
	}
}
func GetUserArticleHistoryCache(deps redis_service.Deps, articleID, userID int) bool {
	if deps.Client == nil {
		return false
	}
	key := fmt.Sprintf("user_history_%d", userID)
	field := fmt.Sprintf("%d", articleID)

	_, err := deps.Client.HGet(context.Background(), key, field).Result()
	if err != nil {
		return false
	}
	return true
}

// 访客阅读记录
func SetGuestArticleHistoryCache(deps redis_service.Deps, articleID int, hash string) {
	if deps.Client == nil {
		return
	}
	key := fmt.Sprintf("guest_history_%s", hash)
	field := fmt.Sprintf("%d", articleID)

	now := time.Now()
	nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())

	if err := deps.Client.HSet(context.Background(), key, field, "").Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("写入访客阅读历史缓存失败: 错误=%v", err)
		}
		return
	}

	if err := deps.Client.ExpireAt(context.Background(), key, nextDay).Err(); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("设置访客阅读历史缓存过期时间失败: 错误=%v", err)
		}
		return
	}
}
func GetGuestArticleHistoryCache(deps redis_service.Deps, articleID int, hash string) bool {
	if deps.Client == nil {
		return false
	}
	key := fmt.Sprintf("guest_history_%s", hash)
	field := fmt.Sprintf("%d", articleID)

	_, err := deps.Client.HGet(context.Background(), key, field).Result()
	if err != nil {
		return false
	}
	return true
}
