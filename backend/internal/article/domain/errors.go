package domain

import "myblogx/internal/apperr"

// 领域错误。
//
// 这里只放错误码，中文文案统一在 internal/apperr 的消息表里。
// 这样做的原因：改造前全项目 125 处 errors.New("中文")，前端只能匹配
// 中文字符串；错误码是稳定契约，文案可以随便改。
//
// 用法：
//
//	if errors.Is(err, domain.ErrArticleNotFound) { ... }   // 或 apperr.IsCode(err, apperr.CodeArticleNotFound)
var (
	// ErrArticleNotFound 文章不存在，或对当前操作者不可见。
	// 两者故意合并：否则可以通过"是否返回不存在"探测他人是否有草稿。
	ErrArticleNotFound = apperr.New(apperr.CodeArticleNotFound)
	// ErrForbidden 无权执行该操作。
	ErrForbidden = apperr.New(apperr.CodeArticleForbidden)
	// ErrInvalidTransition 非法状态迁移。
	ErrInvalidTransition = apperr.New(apperr.CodeArticleInvalidState)
	// ErrAlreadyPublished 已发布文章不能重复发布。
	ErrAlreadyPublished = apperr.New(apperr.CodeArticleAlreadyPublic)
	// ErrTitleRequired 标题不能为空。
	ErrTitleRequired = apperr.New(apperr.CodeArticleTitleRequired)
	// ErrContentRequired 正文不能为空。
	ErrContentRequired = apperr.New(apperr.CodeArticleContentRequired)
	// ErrVersionConflict 并发修改冲突（乐观锁）。
	ErrVersionConflict = apperr.New(apperr.CodeArticleVersionConflict)
	// ErrPublishDisabledBySite 站点策略关闭了非管理员的发布能力。
	//
	// 与 ErrForbidden 分开：后者是"你无权"，前者是"站点当前不允许发布"，
	// 文案里要能看出该找管理员改开关（ADR-0017 决策 2）。
	ErrPublishDisabledBySite = apperr.New(apperr.CodeArticlePublishDisabled)
	// ErrDraftDisabledBySite 站点策略关闭了非管理员的存草稿能力。
	ErrDraftDisabledBySite = apperr.New(apperr.CodeArticleDraftDisabled)
)
