package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel/ctype"
)

// BatchReviewItem 是批量审核里一条失败项。
//
// 只带 ID 与错误码，不带错误原文：批量接口的响应要能直接给前端渲染，
// 文案由错误码在 apperr 一处映射（与单条接口同一口径）。
type BatchReviewItem struct {
	TaskID ctype.ID
	Code   apperr.Code
}

// BatchReviewResult 是批量审核的结果。
//
// 部分成功是**正常结局**：一条任务可能因为文章已被别人处理而失败，
// 不该让整批回滚——管理员点一次，能处理的都处理掉，剩下的给出清单。
type BatchReviewResult struct {
	Succeeded int
	Failed    []BatchReviewItem
}

// BatchHandleReviewTasks 批量通过或驳回审核任务（票 313 上游 spec 决策 32）。
//
// 逐条走与单条完全相同的用例（HandleReviewTask），每条自带事务：
// 批量不是新的判定逻辑，只是"把同一次决定重复应用"——共用同一条路径，
// 才不会出现"单条能过、批量不能过"的第二种规则。
func (s *Service) BatchHandleReviewTasks(
	ctx context.Context,
	viewer domain.Viewer,
	taskIDs []ctype.ID,
	approve bool,
	code review.ReasonCode,
	note string,
) (BatchReviewResult, error) {
	if !viewer.IsAdmin() {
		return BatchReviewResult{}, domain.ErrForbidden
	}
	if len(taskIDs) == 0 {
		return BatchReviewResult{}, apperr.New(apperr.CodeCommonInvalidArgument)
	}

	// 原因合法性先校验一次：驳回原因不合法时整批都做不成，没必要逐条撞。
	if _, _, err := reviewReason(approve, code, note); err != nil {
		return BatchReviewResult{}, err
	}

	result := BatchReviewResult{Failed: make([]BatchReviewItem, 0)}
	for _, taskID := range taskIDs {
		_, err := s.HandleReviewTask(ctx, viewer, taskID, approve, code, note)
		if err != nil {
			code, ok := apperr.CodeOf(err)
			if !ok {
				code = apperr.CodeCommonServiceFailed
			}
			result.Failed = append(result.Failed, BatchReviewItem{TaskID: taskID, Code: code})
			continue
		}
		result.Succeeded++
	}
	return result, nil
}
