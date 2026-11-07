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
	// ErrReviewReasonInvalid 驳回原因不是固定枚举里的一条。
	//
	// 驳回原因现在是"固定枚举 + 可选自由文本"（#308）：枚举是结论，自由文本只是补充。
	// 没有枚举就分不出"哪一类问题"，前端也无法按值渲染文案——只认值，不认中文。
	ErrReviewReasonInvalid = apperr.New(apperr.CodeArticleReviewReasonInvalid)
	// ErrReviewReasonRequired 选「其他原因」时没补那句说明。
	//
	// 原因不是"审核操作的附属备注"，而是**送给作者看的正文**：只给一个"其他原因"
	// 等于什么都没说（与 #303 修的是同一件事，只是现在由枚举兜住"必须写"这条底线）。
	ErrReviewReasonRequired = apperr.New(apperr.CodeArticleReviewReasonRequired)
	// ErrReviewReasonTooLong 补充说明超出长度上限。
	//
	// 原因列是 varchar(500)、枚举值也编在同一列（零 DDL），不拦截会变成写库失败。
	ErrReviewReasonTooLong = apperr.New(apperr.CodeArticleReviewReasonTooLong)
	// ErrSubmitQuotaExceeded 「投稿配额」用完：当天已提交的次数达到上限。
	//
	// 与 ErrForbidden / ErrPublishDisabledBySite 分开：那两个要用户改账号或找管理员，
	// 这个只需要等到明天（配额按自然日重置，ADR-0034）。
	ErrSubmitQuotaExceeded = apperr.New(apperr.CodeArticleSubmitQuotaExceeded)
)
