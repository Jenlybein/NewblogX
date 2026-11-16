// 机审评分派生层的装配（票 311）。
//
// 合成根只做跨模块接线：article 用例层定义端口，这里把它接到 ai 模块的评分仓储。
// article 模块本身不认识 AI 模块的实体（R11）。
//
// 词表那一层已随「文章词表机审退场」删除（票 401 / ADR-0032 决策 6），所以这里
// 不再有"现读词表闭包"的接线。
package app

import (
	"context"

	"gorm.io/gorm"

	aimysql "myblogx/internal/ai/adapter/mysql"
	aidomain "myblogx/internal/ai/domain"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
)

// newArticleModerationScores 把 ai 模块的评分仓储接成 article 的只读端口。
//
// 只读最近一条评分记录，不触发模型调用（票 311 验收标准 3）。
func newArticleModerationScores(db *gorm.DB) articleapp.ModerationScorePort {
	return articleModerationScores{repo: aimysql.New(db)}
}

// articleModerationScores 实现 app.ModerationScorePort。
type articleModerationScores struct {
	repo aidomain.ScoreRepository
}

var _ articleapp.ModerationScorePort = articleModerationScores{}

// LatestScores 批量取每篇文章的最近一条评分记录。
//
// ai 仓储的 Latest 是单条的（评分是按篇写的），这里循环而不是给 ai 端口加一个
// 批量方法：批量读取只是审核列表的便利，不值得为它扩大 AI 模块的持久化契约。
// 没有记录的文章跳过，不返回零值假记录。
func (s articleModerationScores) LatestScores(ctx context.Context, articleIDs []domain.ArticleID) (map[domain.ArticleID]moderation.ScoreFacts, error) {
	// 同一页里文章 ID 不会重复（每篇只有一个待处理任务），但仍按集合语义去重，
	// 避免将来复用时对同一篇反复查库。
	out := make(map[domain.ArticleID]moderation.ScoreFacts, len(articleIDs))
	for _, id := range articleIDs {
		if _, ok := out[id]; ok {
			continue
		}
		record, err := s.repo.Latest(ctx, id.Uint64())
		if err != nil {
			return nil, err
		}
		if record == nil {
			continue
		}
		out[id] = moderation.ScoreFacts{
			TotalScore:     record.TotalScore,
			DimensionsJSON: record.DimensionsJSON,
			ContentLength:  record.ContentLength,
		}
	}
	return out, nil
}
