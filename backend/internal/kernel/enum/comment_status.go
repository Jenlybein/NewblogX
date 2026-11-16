package enum

// CommentStatus 是 comment_models.status 的取值。
//
// 只有已发布 / 已删除（票 400）：评论不再有「审核中」，值 1 是退场后的保留值，
// 存量行在启动时由一次性迁移改成已删除。
type CommentStatus int8

const (
	CommentStatusPublished CommentStatus = 2
	CommentStatusDeleted   CommentStatus = 3
)
