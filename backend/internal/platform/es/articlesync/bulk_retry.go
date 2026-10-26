package articlesync

// 批量写的失败处理与重试。
//
// # 背景
//
// ES 的 bulk 是"HTTP 200 + 逐条结果"：一个请求里 127 条成功、1 条因 mapping 冲突失败，
// 状态码完全正常。这类失败必须逐条看，而解析一度散在各调用方 ——
// 有的只回一句 "bulk errors"（不知道哪篇、为什么），有的根本没判断。
//
// 现在解析统一在 es_service.IndexBulk（返回 BulkResult），本文件负责两件业务判断：
//
//  1. **哪几条值得再发一次**：429 / 5xx 是"服务端暂时不行"，重试有意义；
//     4xx 是"这条本身不行"（字段类型不符、mapping 冲突），重发一百次也一样，
//     应该直接进死信让人看。
//  2. **只重发失败的那几条**，不整批重发。bulk 的失败粒度本来就是逐条，
//     整批重发会把成功的再写一遍（幂等但浪费）。
//
// 重试是在 canal 回调里同步做的，会阻塞后续事件。所以次数与间隔都要小：
// 默认 2 次 × 200ms，最坏多花约 400ms（加上 bulk 本身耗时）。
// 真正的兜底是死信 + 按主键重算，不是这里。

import (
	"fmt"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/es"

	"github.com/elastic/go-elasticsearch/v7"
)

// bulkRetryPolicy 是批量写的重试策略（传参而不是包级变量：
// 它只有两个调用点，没必要为它引入一份"启动时注入、之后只读"的全局状态）。
//
// 暂不做成配置项：现在没有真实 429 的证据，先按项目里已有的口径
// （图片引用链路也是 2 次 / 200ms）。真需要调的时候再进配置。
type bulkRetryPolicy struct {
	MaxAttempts int           // 含首次
	Delay       time.Duration // 两次尝试之间的固定等待
}

var defaultBulkRetryPolicy = bulkRetryPolicy{MaxAttempts: 2, Delay: 200 * time.Millisecond}

// bulkWriteOutcome 是一次批量写的最终结论。
type bulkWriteOutcome struct {
	// failures 是**最终仍然失败**的条目（重试后仍失败，或本身不可重试）。
	failures []articleBulkFailure
	// missingIDs 是"update 时文档已不存在"的文章 id，调用方负责补建。
	missingIDs []ctype.ID
	// resp 是最后一次 bulk 的原始响应（用于传输层失败的文案）。
	resp es_service.ESResponse
	// attempts 实际尝试了几次，写进错误信息，便于区分"一次就挂"和"重试也没救"。
	attempts int
}

// bulkWriteWithRetry 发送一批 bulk 请求，按策略重试可重试的部分。
func bulkWriteWithRetry(client *elasticsearch.Client, reqs []*es_service.BulkRequest, maxItems int, policy bulkRetryPolicy) bulkWriteOutcome {
	var out bulkWriteOutcome
	if len(reqs) == 0 {
		out.resp = es_service.ESResponse{Success: true}
		return out
	}

	pending := reqs
	for out.attempts < policy.MaxAttempts && len(pending) > 0 {
		out.attempts++
		if out.attempts > 1 && policy.Delay > 0 {
			time.Sleep(policy.Delay)
		}

		out.resp = es_service.IndexBulk(client, IndexName(), pending)
		result, _ := out.resp.Data.(es_service.BulkResult)

		failures, missing := collectArticleBulkFailures(result, pending, maxItems)
		out.failures = failures
		out.missingIDs = append(out.missingIDs, missing...)

		if out.resp.Success && len(failures) == 0 {
			return out
		}
		pending = retryableRequests(pending, result)
	}
	return out
}

// bulkWriteError 把一次批量写的结论翻译成错误（nil = 这一批最终没有问题）。
//
// 三种情况要分开，混在一起就会误报：
//   - 还有遗留失败 → 报失败明细（哪几条、什么原因），调用方据此落死信；
//   - 有逐条结果但失败已被业务处理掉（update 命中 404 → 调用方已回源补建）→ 成功；
//   - 没有逐条结果（传输层/整请求失败）→ 用原始 Msg，说明"整批没发出去"。
//
// 中间那条最容易写错：IndexBulk 变诚实之后，resp.Success 会是 false，
// 若只看它就会把"已经补建成功"也报成失败。
func bulkWriteError(out bulkWriteOutcome, errPrefix string) error {
	if len(out.failures) > 0 {
		return fmt.Errorf("%s: %s（尝试 %d 次）", errPrefix, formatArticleBulkFailureSummary(out.failures), out.attempts)
	}
	if out.resp.Success || out.resp.Data != nil {
		return nil
	}
	return fmt.Errorf("%s: %s（尝试 %d 次）", errPrefix, out.resp.Msg, out.attempts)
}

// retryableRequests 挑出值得再发一次的请求子集。
//
// 没有逐条信息（result 为空）= 整个请求失败（网络中断、索引不存在、429 被整体拒绝），
// 这时只能整批重试 —— 至多一次，代价可控。
func retryableRequests(reqs []*es_service.BulkRequest, result es_service.BulkResult) []*es_service.BulkRequest {
	if !result.HasFailure() {
		return reqs
	}

	picked := make([]*es_service.BulkRequest, 0, len(result.Failures))
	seen := make(map[int]struct{}, len(result.Failures))
	for _, failure := range result.Failures {
		if !retryableBulkStatus(failure.Status) {
			continue
		}
		if failure.Index < 0 || failure.Index >= len(reqs) || reqs[failure.Index] == nil {
			continue
		}
		if _, ok := seen[failure.Index]; ok {
			continue
		}
		seen[failure.Index] = struct{}{}
		picked = append(picked, reqs[failure.Index])
	}
	return picked
}

// retryableBulkStatus 判断单条失败是否值得重试。
//
// 429 限流、5xx 服务端问题：换个时间点可能就好了。
// 4xx 是"这条数据本身不允许"（mapping 冲突、字段类型不符、文档过大），
// 重试只会把同样的错误再拿一次。
func retryableBulkStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
