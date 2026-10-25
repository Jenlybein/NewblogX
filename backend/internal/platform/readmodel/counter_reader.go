package readmodel

import (
	"context"
	"fmt"
	"strconv"

	"myblogx/internal/kernel/ctype"
	redis_service "myblogx/internal/platform/redis"
)

type CommentBatchCounters struct {
	ReplyMap map[ctype.ID]int
	DiggMap  map[ctype.ID]int
}

type CommentCounterReader struct {
	Cache redis_service.Options
}

func NewCommentCounterReader(cacheOptions redis_service.Options) CommentCounterReader {
	return CommentCounterReader{
		Cache: cacheOptions,
	}
}

func (r CommentCounterReader) Batch(commentIDs []ctype.ID) CommentBatchCounters {
	counters := CommentBatchCounters{
		ReplyMap: make(map[ctype.ID]int),
		DiggMap:  make(map[ctype.ID]int),
	}
	if r.Cache.Client == nil || len(commentIDs) == 0 {
		return counters
	}

	ctx := context.Background()
	fields := buildBatchFields(commentIDs)
	pipe := r.Cache.Client.Pipeline()
	defer pipe.Close()

	replyCmd := pipe.HMGet(ctx, "comment_reply", fields...)
	diggCmd := pipe.HMGet(ctx, "comment_digg", fields...)

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

func buildBatchFields(ids []ctype.ID) []string {
	fields := make([]string, 0, len(ids))
	for _, id := range ids {
		fields = append(fields, id.String())
	}
	return fields
}

func decodeBatchValues(ids []ctype.ID, values []any) map[ctype.ID]int {
	result := make(map[ctype.ID]int, len(ids))
	for i, raw := range values {
		if raw == nil || i >= len(ids) {
			continue
		}
		num, err := strconv.Atoi(fmt.Sprint(raw))
		if err != nil {
			continue
		}
		result[ids[i]] = num
	}
	return result
}
