package read_repo

import (
	"context"
	"fmt"
	"strconv"

	"myblogx/models/ctype"
	"myblogx/platform/cachex"
)

type ArticleBatchCounters struct {
	ViewMap    map[ctype.ID]int
	DiggMap    map[ctype.ID]int
	FavorMap   map[ctype.ID]int
	CommentMap map[ctype.ID]int
}

type ArticleCounterReader struct {
	Cache cachex.Deps
}

func NewArticleCounterReader(cacheDeps cachex.Deps) ArticleCounterReader {
	return ArticleCounterReader{
		Cache: cacheDeps,
	}
}

func (r ArticleCounterReader) Batch(articleIDs []ctype.ID) ArticleBatchCounters {
	counters := ArticleBatchCounters{
		ViewMap:    make(map[ctype.ID]int),
		DiggMap:    make(map[ctype.ID]int),
		FavorMap:   make(map[ctype.ID]int),
		CommentMap: make(map[ctype.ID]int),
	}
	if r.Cache.Client == nil || len(articleIDs) == 0 {
		return counters
	}

	ctx := context.Background()
	fields := buildBatchFields(articleIDs)
	pipe := r.Cache.Client.Pipeline()
	defer pipe.Close()

	viewCmd := pipe.HMGet(ctx, "article_view", fields...)
	diggCmd := pipe.HMGet(ctx, "article_digg", fields...)
	favorCmd := pipe.HMGet(ctx, "article_favorite", fields...)
	commentCmd := pipe.HMGet(ctx, "article_comment", fields...)

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

type CommentBatchCounters struct {
	ReplyMap map[ctype.ID]int
	DiggMap  map[ctype.ID]int
}

type CommentCounterReader struct {
	Cache cachex.Deps
}

func NewCommentCounterReader(cacheDeps cachex.Deps) CommentCounterReader {
	return CommentCounterReader{
		Cache: cacheDeps,
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
