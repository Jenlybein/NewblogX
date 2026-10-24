package domain

import "errors"

// 领域错误。
//
// 约定：错误文案面向最终用户（HTTP adapter 会直接把它转成响应消息），
// 因此不再包一层"操作失败"，避免出现"更新文章失败: 无权修改他人文章"这种叠加。
var (
	// ErrArticleNotFound 文章不存在，或对当前操作者不可见。
	// 两者故意合并：否则可以通过"是否返回不存在"探测他人是否有草稿。
	ErrArticleNotFound = errors.New("文章不存在")
	// ErrForbidden 无权执行该操作。
	ErrForbidden = errors.New("无权操作该文章")
	// ErrInvalidTransition 非法状态迁移。
	ErrInvalidTransition = errors.New("当前状态不允许该操作")
	// ErrAlreadyPublished 已发布文章不能重复发布。
	ErrAlreadyPublished = errors.New("文章已发布")
	// ErrTitleRequired 标题不能为空。
	ErrTitleRequired = errors.New("标题不能为空")
	// ErrContentRequired 正文不能为空。
	ErrContentRequired = errors.New("正文不能为空")
	// ErrVersionConflict 并发修改冲突（乐观锁）。
	ErrVersionConflict = errors.New("文章已被他人修改，请刷新后重试")

	// 子域错误。
	// ErrFavoriteExists 已收藏过。
	ErrFavoriteExists = errors.New("已收藏该文章")
	// ErrFavoriteNotFound 尚未收藏。
	ErrFavoriteNotFound = errors.New("尚未收藏该文章")
	// ErrSelfFavorite 不能收藏自己的文章。
	ErrSelfFavorite = errors.New("不能收藏自己的文章")
	// ErrTopLimitExceeded 置顶数量超限。
	ErrTopLimitExceeded = errors.New("置顶数量已达上限")
	// ErrTagDisabled 标签不存在或已停用。
	ErrTagDisabled = errors.New("标签不存在或已停用")
	// ErrCategoryNotFound 分类不存在或不属于该用户。
	ErrCategoryNotFound = errors.New("分类不存在")
	// ErrReviewTaskNotFound 审核任务不存在。
	ErrReviewTaskNotFound = errors.New("审核任务不存在")
	// ErrReviewTaskHandled 审核任务已处理。
	ErrReviewTaskHandled = errors.New("审核任务已处理")
)
