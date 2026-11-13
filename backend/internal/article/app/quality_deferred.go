package app

import (
	"context"
	"encoding/json"
	"errors"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// evaluateQuality 是闸门判定的纯规则部分（ADR-0023 的阈值 + 木桶口径）。
//
// 同步链路（judgeQuality）和异步链路（AssessDeferredQuality）共用它：
// 两条链路只是"分数从哪来"不同，判定规则必须只有一处。
func evaluateQuality(
	policy QualityGatePolicy, total int, dims []QualityDimension, issues []QualityIssue,
) qualityVerdict {
	required := policy.PublishLine()
	weak := weakDimensions(dims, policy.PassScore)

	// 木桶：总分过线 **且** 六项都过及格线。两个条件都要成立 ——
	// 只留其中一个都会出现"六项都 51 与六项都 90 同等放行"这类说不清的口径。
	if total >= required && len(weak) == 0 {
		return qualityVerdict{Passed: true, Score: total, Required: required}
	}

	gap := required - total
	if gap < 0 {
		// 总分过线、被维度卡住时，差额不是"还差几分"，不能给负数让作者误解。
		gap = 0
	}
	return qualityVerdict{
		Passed:         false,
		Score:          total,
		Required:       required,
		Gap:            gap,
		Level:          policy.Level(total),
		WeakDimensions: weak,
		Issues:         issues,
	}
}

// AssessDeferredQuality 用**已有的评分记录**做异步判定（ADR-0030）。
//
// 它不做任何模型调用：评分由发件箱的 scoreSubscriber 先行生成，这里只读结论。
// 没有评分记录或端口缺失一律按"拿不到分数"报错——异步链路失败关闭：文章继续
// 停在「已发布待可见」，发件箱重试；重试耗尽也不能当干净放行。
func (s *Service) AssessDeferredQuality(
	ctx context.Context, articleID domain.ArticleID, policy QualityGatePolicy,
) (QualityAssessment, error) {
	if !policy.Enabled {
		return QualityAssessment{Passed: true}, nil
	}
	if s.scores == nil {
		return QualityAssessment{}, apperr.New(apperr.CodeArticleScoreUnavailable)
	}

	facts, err := s.scores.LatestScores(ctx, []domain.ArticleID{articleID})
	if err != nil {
		return QualityAssessment{}, apperr.Wrap(apperr.CodeArticleScoreUnavailable, err)
	}
	fact, ok := facts[articleID]
	if !ok {
		return QualityAssessment{}, apperr.New(apperr.CodeArticleScoreUnavailable)
	}
	v := evaluateQuality(policy, fact.TotalScore, parseScoreDimensions(fact.DimensionsJSON), nil)
	return QualityAssessment{
		Passed:         v.Passed,
		Score:          v.Score,
		Required:       v.Required,
		Gap:            v.Gap,
		Level:          v.Level,
		WeakDimensions: v.WeakDimensions,
	}, nil
}

// parseScoreDimensions 解析评分记录里的六维分数；空或坏数据按"没有维度分"处理。
//
// 坏数据不报错：维度分缺失时弱项判定为空，仍按总分判（与引入木桶之前一致）。
// 让一条脏 JSON 把整个提交流程判成失败，代价比"少看一眼短板"大得多。
func parseScoreDimensions(raw string) []QualityDimension {
	if raw == "" || raw == "null" {
		return nil
	}
	var items []struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	out := make([]QualityDimension, 0, len(items))
	for _, item := range items {
		out = append(out, QualityDimension{Name: item.Name, Score: item.Score})
	}
	return out
}

// DemotePendingForQuality 把异步质量判定未达标的「已发布待可见」文章退回草稿。
//
// 返回 demoted=false 表示文章已经不在"等判定"这一档（作者又改过、已被合规
// 处理、或重复投递），不需要也不该再动它。
func (s *Service) DemotePendingForQuality(
	ctx context.Context, articleID domain.ArticleID, reason string,
) (bool, error) {
	var demoted bool
	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		a, err := r.Articles().Find(ctx, articleID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := a.DemoteForQuality(now); err != nil {
			if errors.Is(err, domain.ErrInvalidTransition) {
				// 已经被别的路径处理过：这不是失败，什么都不做。
				return nil
			}
			return err
		}
		if err := cancelPendingReviewTasks(ctx, r, a.ID(), reason, now); err != nil {
			return err
		}
		if err := r.Articles().Update(ctx, a); err != nil {
			return err
		}
		demoted = true
		return nil
	})
	return demoted, err
}

// QualityAssessment 是一次异步质量判定的对外结论。
//
// 发件箱链路拿它拼通知文案（分数 / 放行线），所以必须是导出类型；
// 判定内部仍然走未导出的 qualityVerdict，两者只在这个映射处发生接触。
type QualityAssessment struct {
	Passed         bool
	Score          int
	Required       int
	Gap            int
	Level          string
	WeakDimensions []QualityDimension
}
