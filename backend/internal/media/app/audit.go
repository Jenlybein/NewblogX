package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/media/domain"
)

// auditTimeoutStatuses 是"还没有结论"的状态集合。
//
// 只有这两种状态会被超时扫描推走：Pending（连结论都没有）与 Reviewing
// （机审疑似、等人工）。其余状态已是终态，扫到也不该动。
func auditTimeoutStatuses() []domain.Status {
	return []domain.Status{domain.StatusPending, domain.StatusReviewing}
}

// CensorNow 主动对一张图跑一次内容审核，并把结论落库。
//
// 这是"审核结论"的第二条来源（第一条是七牛回调）。它存在的理由很直接：
// 回调需要公网可达的回调地址，域名被封或地址配错时永远等不到；
// 而这条是出站请求，本机就能拿到结论。
//
// 调用失败时**不改状态**：图继续停在待审核，由超时扫描转人工队列。
// 绝不因为"审核没跑起来"就放行。
func (s *Service) CensorNow(ctx context.Context, image *domain.Image) error {
	if !s.cfg().AuditEnabled || image == nil || s.storage == nil {
		return nil
	}
	suggestion, err := s.storage.Censor(ctx, image.Bucket, image.ObjectKey)
	if err != nil {
		if s.logger != nil {
			s.logger.Warnf("主动审核失败（图保持待审核）: image=%d key=%s err=%v",
				image.ID, image.ObjectKey, err)
		}
		return err
	}
	status := domain.StatusFromSuggestion(suggestion)
	if !canTransition(image.Status, status) {
		return nil
	}
	return s.applyStatus(ctx, image, status, auditSourceActive)
}

// ApproveAudit 管理员人工放行一张疑似图片。
//
// 只允许从 Reviewing / Pending 放行：
//   - Blocked 是终局（对象已删），"放行"没有意义，也不能让它看起来可用；
//   - Pass 本来就可展示，重复放行没有意义。
//
// 放行时若存储侧曾被禁用，这里一并解禁 —— 否则应用层说"可展示"、
// 存储层仍返回 403，用户看到的是裂图。
func (s *Service) ApproveAudit(ctx context.Context, id domain.ImageID) (*domain.Image, error) {
	image, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch image.Status {
	case domain.StatusReviewing, domain.StatusPending:
	case domain.StatusBlocked:
		return nil, apperr.NewWithMessage(apperr.CodeImageAuditInvalid,
			"该图已被判定违规（对象已删除），不能放行")
	default:
		return nil, apperr.NewWithMessage(apperr.CodeImageAuditInvalid, "当前状态无需放行")
	}

	if err := s.repo.UpdateStatus(ctx, image.ID, domain.StatusPass); err != nil {
		return nil, err
	}
	image.Status = domain.StatusPass

	// 解禁放行：只有存储侧也开着，图才真的能显示。
	if err := s.storage.SetEnabled(ctx, image.Bucket, image.ObjectKey, true); err != nil {
		if s.logger != nil {
			s.logger.Warnf("放行时解禁对象失败（图已标为通过）: image=%d key=%s err=%v",
				image.ID, image.ObjectKey, err)
		}
	}
	return &image, nil
}

// RejectAudit 管理员驳回一张图（判定违规）。
//
// 与回调判定违规走同一条副作用链：状态置 blocked（终局）+ 按开关删对象。
func (s *Service) RejectAudit(ctx context.Context, id domain.ImageID) (*domain.Image, error) {
	image, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if image.Status == domain.StatusBlocked {
		return nil, apperr.NewWithMessage(apperr.CodeImageAuditInvalid, "该图已被判定违规")
	}
	if err := s.applyStatus(ctx, &image, domain.StatusBlocked, auditSourceManual); err != nil {
		return nil, err
	}
	return &image, nil
}

// SweepAuditTimeout 把"等太久还没结论"的图转进人工队列。
//
// 参数 before 是截止时间：早于它入库、且仍在待审核/疑似的图会被转成 Reviewing。
// 返回处理条数。
//
// 为什么转人工而不是放行：审核通道故障时唯一安全的动作是"让人看"。
// 放行等于把没审过的图当审过，那正是这次要修的问题。
func (s *Service) SweepAuditTimeout(ctx context.Context, beforeUnix int64) (int, error) {
	if !s.cfg().AuditEnabled {
		return 0, nil
	}
	// 这里刻意不去碰 domain.Query 的分页：它按 id 倒序、一次最多 100 条，
	// 对一个"每分钟跑一次、只处理积压"的任务够用；积压更多时下一轮继续。
	q := domain.Query{Page: 1, Limit: 100}
	moved := 0
	for _, status := range auditTimeoutStatuses() {
		current := status
		q.Status = &current
		images, _, err := s.repo.List(ctx, q)
		if err != nil {
			return moved, err
		}
		for i := range images {
			image := images[i]
			if image.CreatedAt.Unix() >= beforeUnix {
				continue
			}
			if err := s.concurrentSafeMarkReviewing(ctx, &image); err != nil {
				if s.logger != nil {
					s.logger.Warnf("超时转人工失败: image=%d err=%v", image.ID, err)
				}
				continue
			}
			moved++
		}
	}
	return moved, nil
}

// concurrentSafeMarkReviewing 把图推向人工队列，并把存储侧对象禁用。
func (s *Service) concurrentSafeMarkReviewing(ctx context.Context, image *domain.Image) error {
	if image.Status == domain.StatusReviewing {
		// 已经是人工队列；补一次禁用（幂等），避免上一轮禁用失败后没人再管。
		return s.storage.SetEnabled(ctx, image.Bucket, image.ObjectKey, false)
	}
	if err := s.repo.UpdateStatus(ctx, image.ID, domain.StatusReviewing); err != nil {
		return err
	}
	image.Status = domain.StatusReviewing
	if err := s.storage.SetEnabled(ctx, image.Bucket, image.ObjectKey, false); err != nil {
		if s.logger != nil {
			s.logger.Warnf("疑似图禁用对象失败（图已进人工队列）: image=%d key=%s err=%v",
				image.ID, image.ObjectKey, err)
		}
	}
	return nil
}
