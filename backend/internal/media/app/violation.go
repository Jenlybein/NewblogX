package app

import (
	"context"

	"myblogx/internal/media/domain"
)

// ViolationRecorder 是"图片违规 → 违规计分"的窄端口（ADR-0035）。
//
// 为什么不直接 import 计分模块：媒体模块不该知道"分数"这回事。
// 它只负责说"这张图被判违规了、属于谁"，怎么记分是计分模块的内部事。
type ViolationRecorder interface {
	RecordImageViolation(ctx context.Context, userID, imageID uint64, objectKey string) error
}

// SetViolationRecorder 注入违规计分端口。
//
// 装配层在启动时调用一次；为 nil 时"判定违规"照常生效，只是不记分
// （测试与最小装配不需要计分）。
func (s *Service) SetViolationRecorder(recorder ViolationRecorder) {
	s.violations = recorder
}

// recordViolation 在判定违规时上报一次计分（失败不影响状态变更）。
func (s *Service) recordViolation(ctx context.Context, image *domain.Image) {
	if s.violations == nil || image == nil || image.UserID.Uint64() == 0 {
		return
	}
	if err := s.violations.RecordImageViolation(ctx, image.UserID.Uint64(), image.ID.Uint64(), image.ObjectKey); err != nil {
		if s.logger != nil {
			s.logger.Warnf("上报图片违规计分失败: image=%d user=%d err=%v", image.ID, image.UserID, err)
		}
	}
}
