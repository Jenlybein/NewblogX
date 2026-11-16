// Package app 是内容过滤模块的用例层：命中写入（Filter / Record）与风险页读面。
//
// 失败方向是这一层最重要的契约（ADR-0032 决策 11）：
//   - 引擎没装配时原样返回文本（退化，不阻断用户发布）；
//   - 命中记录写失败只记 error 日志，替换后的文本照常返回给调用方。
//
// 也就是说"审计写不进去"永远不该演变成"用户发不出评论"。
package app

import (
	"context"
	"time"

	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// TopUserLimit 是风险页顶部"命中用户 Top"展示的条数。
//
// 固定 5 条而不是让前端传：这是页面上的一个固定运营位，不是可筛选列表；
// 开放成参数只会让"每页都多查一次"变得可控的同时没人用。
const TopUserLimit = 5

// Service 是内容过滤应用服务。
type Service struct {
	// engine 是全站唯一的过滤引擎（启动时构建，词表保存后原子重建）。
	// nil 表示装配遗漏：Filter 退化成"原样返回"，不让整站不可用。
	engine *contentfilter.Engine
	hits   domain.Repository
	logger domain.Logger
}

// New 构造内容过滤服务。
func New(engine *contentfilter.Engine, hits domain.Repository, logger domain.Logger) *Service {
	return &Service{engine: engine, hits: hits, logger: logger}
}

// FilterInput 是"过滤一段文本"的输入。
type FilterInput struct {
	Surface domain.Surface
	UserID  ctype.ID
	// ContentID 是这条文本最终落库的 ID。评论在 Create 之前先生成新评论 ID，
	// 所以记录能指回真实内容（#398 的处置动作要靠它）。
	ContentID ctype.ID
	Text      string
}

// FilterOutput 是过滤结果。
type FilterOutput struct {
	// Masked 是替换后的文本（与输入等长，按 rune 计）。
	Masked string
	// FilteredCount 是实际被替换的处数（白名单命中不算，重叠命中按并集算一处）。
	FilteredCount int
}

// Filter 过滤文本并记录命中（写入口）。
//
// 返回的 error 只有"引擎真的出问题"这类装配级异常；记录写失败**不在其中** ——
// 那条路径只记日志（见 Record）。调用方拿到 error 时应当记日志并按原文继续，
// 而不是拒绝用户发布。
func (s *Service) Filter(ctx context.Context, in FilterInput) (FilterOutput, error) {
	if s.engine == nil {
		return FilterOutput{Masked: in.Text}, nil
	}
	result := s.engine.Filter(in.Text)
	out := FilterOutput{Masked: result.Masked, FilteredCount: result.MaskedSpans}
	if len(result.Hits) == 0 {
		return out, nil
	}
	s.Record(ctx, domain.Hit{
		CreatedAt: time.Now(),
		Surface:   in.Surface,
		ContentID: in.ContentID,
		UserID:    in.UserID,
		Words:     toWords(result.Hits),
		HitCount:  countSnippets(result.Hits),
		Status:    domain.StatusPending,
	})
	return out, nil
}

// Record 写入一条命中记录；写失败只记日志，不向调用方返回错误。
//
// 单独暴露成方法而不是只藏在 Filter 里：存量清洗（后续票的 -filter-sweep）要按
// 自己的节奏补写记录，它不该为了写一条记录再过滤一遍文本。
func (s *Service) Record(ctx context.Context, hit domain.Hit) {
	if s.hits == nil {
		return
	}
	if err := s.hits.Create(ctx, &hit); err != nil {
		s.logError(
			"写入关键词命中记录失败（不阻断主流程）: surface=%s content=%d user=%d words=%d err=%v",
			hit.Surface, hit.ContentID.Uint64(), hit.UserID.Uint64(), len(hit.Words), err,
		)
	}
}

// ListInput 是风险页列表的输入。Surface / Status 为空表示不按该项筛。
type ListInput struct {
	Surface domain.Surface
	Status  domain.Status
	UserID  ctype.ID
	Page    int
	Limit   int
}

// ListResult 是风险页一页的数据与顶部命中用户 Top。
type ListResult struct {
	Items    []domain.Hit
	Total    int64
	TopUsers []domain.TopUser
}

// List 返回风险页的一页记录，并顺带算好顶部命中用户 Top。
//
// Top 与列表同一次请求返回：页面打开时它俩总是一起要，分两个端点的唯一效果是
// 首屏多一次往返。Top 自己失败不影响列表（只记日志、返回空）—— 它是个辅助视图，
// 不该让"风险记录看不了"这种后果由它引起。
func (s *Service) List(ctx context.Context, in ListInput) (ListResult, error) {
	if s.hits == nil {
		return ListResult{}, nil
	}
	page := domain.Page{Page: in.Page, Limit: in.Limit}
	rows, total, err := s.hits.List(ctx, domain.Query{
		Surface: in.Surface,
		Status:  in.Status,
		UserID:  in.UserID,
		Page:    page,
	})
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Items: rows, Total: total}
	top, topErr := s.hits.TopUsers(
		ctx, time.Now().AddDate(0, 0, -domain.TopWindowDays), TopUserLimit,
	)
	if topErr != nil {
		s.logError("读取命中用户 Top 失败（风险列表照常返回）: %v", topErr)
		return result, nil
	}
	result.TopUsers = top
	return result, nil
}

// toWords 把引擎的命中明细转成领域形状。
func toWords(hits []contentfilter.Hit) []domain.Word {
	out := make([]domain.Word, 0, len(hits))
	for _, hit := range hits {
		snippets := make([]domain.Snippet, 0, len(hit.Snippets))
		for _, snippet := range hit.Snippets {
			snippets = append(snippets, domain.Snippet{
				Start: snippet.Start,
				End:   snippet.End,
				Text:  snippet.Text,
			})
		}
		out = append(out, domain.Word{
			Word:     hit.Word,
			Tier:     hit.Tier,
			Category: hit.Category,
			Snippets: snippets,
		})
	}
	return out
}

// countSnippets 统计命中处数：同一个词出现多次时每条上下文都算一处。
func countSnippets(hits []contentfilter.Hit) int {
	total := 0
	for _, hit := range hits {
		total += len(hit.Snippets)
	}
	return total
}

// logError 写一条 error 日志（logger 未装配时静默跳过）。
func (s *Service) logError(format string, args ...any) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}
