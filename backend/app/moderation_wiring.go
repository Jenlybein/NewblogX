// 机审前两层的装配（票 311）。
//
// 合成根只做跨模块接线：article 用例层定义端口，这里把它们接到 ai 模块的评分
// 仓储与平台层的运行时配置。article 模块本身不认识 AI 模块的实体，也不 import
// 平台配置（R11）。
package app

import (
	"context"

	"gorm.io/gorm"

	aimysql "myblogx/internal/ai/adapter/mysql"
	aidomain "myblogx/internal/ai/domain"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/platform/config"
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

// articleModerationWords 返回现读词表的闭包。
//
// 每次调用都回到运行时配置的现读快照：运营改完词表，下一次列表请求立即生效，
// 不需要重启，也不需要把旧结论回填（票 311 验收标准 2）。
func articleModerationWords(effective *config.EffectiveConfig) articleapp.ModerationWordProvider {
	return func() moderation.WordLists {
		article := effective.Article()
		return moderation.WordLists{
			Block: article.Moderation.BlockWords,
			Watch: article.Moderation.WatchWords,
		}
	}
}
