package app

import (
	"context"
	"fmt"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// 发布质量闸门（票 306 / ADR-0032）。
//
// 这里是"拿分数做决定"的地方：评分本身由 QualityGatePort 完成（装配层接到
// AI 模块），本文件只决定这次提交落草稿还是继续走发布/审核。
//
// 两条必须守住的纪律：
//  1. **评分调用在数据库事务之外**。一次评分是 5–30 秒的模型调用，占着事务等于
//     钉死连接池。Create / Update 都在 WithinTx 之前调用 judgeQuality；
//     Update 的"这次会不会提交"用一次只读预演（previewUpdate）先算出来。
//  2. **失败关闭但分开两类**：拿不到可用分数是 article.score_unavailable（不落库、
//     归还配额）；拿到分数但不够发布线是 article.quality_rejected（文章落草稿）。
//

// QualityGatePolicy 是一次提交时生效的闸门配置快照。
//
// 由 HTTP 层每请求从运行时配置现读；零值 = 闸门关闭，与引入闸门之前完全一致。
// 及格线与四个档界都**不做交叉校验**（spec 决策 11）：允许保存"及格线 50 而
// 31–59 叫不合格文章"这类配置，作者侧显示档名、闸门只看发布线。
type QualityGatePolicy struct {
	// Enabled 是闸门开关。
	Enabled bool
	// PassScore 是及格线（0–100）。发布线 = 及格线 + 灰带宽度。
	PassScore int
	// GrayBandWidth 是灰带宽度（默认 2）：及格线**之上**再让出这么多分才放行。
	GrayBandWidth int
	// 四个档界是五个档位的上界：较差 / 不合格 / 常规 / 优质 / 精品。
	// 默认 30 / 59 / 80 / 89，与评分提示词里的分档一致。
	BandPoorMax        int
	BandUnqualifiedMax int
	BandNormalMax      int
	BandGoodMax        int
}

// PublishLine 是实际判定用的发布线。
//
// 灰带做在及格线**上方**而不是上下各留一半（spec 决策 9 与 ADR-0032 决策 7）：
// 实际放行线 = 及格线 + 灰带宽度。只有把放行线整体抬出抖动量程，"49 与 51 都卡"
// 的窄缝才不会正好落在阈值正上方；两侧各留一半挡不住阈值附近的抖动。
// 作者看到的"还差几分"按这条线算，三个数字（及格线 / 灰带 / 放行线）后台都可见。
func (p QualityGatePolicy) PublishLine() int {
	return p.PassScore + p.GrayBandWidth
}

// Level 按配置里的四个档界给出档名。
//
// 档名只用于给作者看的文案（spec 决策 10：作者侧照旧显示档名），不参与判定 ——
// 于是"不合格文章 · 55 分 · 已通过"这种自相矛盾显示是被有意接受的代价。
func (p QualityGatePolicy) Level(score int) string {
	switch {
	case score <= p.BandPoorMax:
		return "较差文章"
	case score <= p.BandUnqualifiedMax:
		return "不合格文章"
	case score <= p.BandNormalMax:
		return "常规文章"
	case score <= p.BandGoodMax:
		return "优质文章"
	default:
		return "精品文章"
	}
}

// qualityVerdict 是一次闸门判定的内部结论。
type qualityVerdict struct {
	Passed bool
	Score  int
	// Required 是发布线（及格线 + 灰带宽度）。
	Required int
	// Gap 是离发布线还差几分（Passed 为 false 时才有意义）。
	Gap int
	// Level 是按配置档界算出的档名。
	Level  string
	Issues []QualityIssue
}

// judgeQuality 取分并按配置判定。
//
// 返回的 error 一律表示"没拿到可用分数"（score unavailable）：端口错误、装配
// 缺失都归这里，调用方必须拒绝本次提交并归还配额。端口自己报的"分数低"不是
// 错误，走 qualityVerdict.Passed=false。
func (s *Service) judgeQuality(
	ctx context.Context, policy QualityGatePolicy, in QualityGateInput,
) (qualityVerdict, error) {
	if !policy.Enabled {
		return qualityVerdict{Passed: true}, nil
	}
	if s.quality == nil {
		// 闸门开着却没有评分端口：装配缺失。按"拿不到分数"失败关闭，
		// 而不是静默放行（静默放行等于闸门形同虚设）。
		return qualityVerdict{}, apperr.New(apperr.CodeArticleScoreUnavailable)
	}

	score, err := s.quality.Evaluate(ctx, in)
	if err != nil {
		// 保留原因便于排障，但对外只认 article.score_unavailable。
		return qualityVerdict{}, apperr.Wrap(apperr.CodeArticleScoreUnavailable, err)
	}

	required := policy.PublishLine()
	if score.TotalScore >= required {
		return qualityVerdict{Passed: true, Score: score.TotalScore, Required: required}, nil
	}
	return qualityVerdict{
		Passed:   false,
		Score:    score.TotalScore,
		Required: required,
		Gap:      required - score.TotalScore,
		Level:    policy.Level(score.TotalScore),
		Issues:   score.Issues,
	}, nil
}

// QualityRejectionDetails 是「质量不达标」给作者的补充信息。
//
// 它作为 apperr 的 cause 挂出来：错误码仍在 apperr 一处映射（前端按码分支），
// 而分数、差额与问题列表是动态数据，不能塞进错误码文案表。HTTP 层用
// errors.As 取到它之后以结构化 data 返回，作者因此能看到"差几分、改哪几条"。
type QualityRejectionDetails struct {
	// ArticleID 是落库后的草稿 ID（新建文章也已经落库）。
	ArticleID domain.ArticleID `json:"article_id"`
	// Score 是本次评分。
	Score int `json:"score"`
	// RequiredScore 是发布线（及格线 + 灰带宽度）。
	RequiredScore int `json:"required_score"`
	// Gap 是还差几分。
	Gap int `json:"gap"`
	// Level 是按运行时档界算出的档名。
	Level string `json:"level"`
	// Issues 是逐条问题（可能为空：模型没给问题但分数不够）。
	Issues []QualityIssue `json:"issues"`
}

// Error 实现 error；文案只在日志/兜底提示里用，正式响应由 HTTP 层给结构化 data。
func (d *QualityRejectionDetails) Error() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf(
		"文章已保存为草稿：评分 %d 分（%s），还差 %d 分才达到发布线（%d 分）",
		d.Score, d.Level, d.Gap, d.RequiredScore)
}

// newQualityRejection 把判定结论包成带错误码的错误。
func newQualityRejection(articleID domain.ArticleID, v qualityVerdict) error {
	return apperr.Wrap(apperr.CodeArticleQualityRejected, &QualityRejectionDetails{
		ArticleID:     articleID,
		Score:         v.Score,
		RequiredScore: v.Required,
		Gap:           v.Gap,
		Level:         v.Level,
		Issues:        v.Issues,
	})
}

// updatePreview 是 Update 在事务外评分之前需要的"这次会不会提交"的预演结论。
//
// 只读：previewUpdate 不改库，也不预占配额。它的唯一用途是让评分发生在
// 数据库事务之外（ADR-0032），写事务会重新读一遍文章并以它读到的状态为准。
type updatePreview struct {
	// NeedsSubmit 表示这次更新确实是一次提交动作（显式提交或自动重新送审）。
	// 票 302 新规则：不论闸门开不开，提交都要在事务外先做同步风险扫描，
	// 所以字段从 NeedsGate 改名，语义也放宽了。
	NeedsSubmit bool
	Title       string
	Content     string
}
