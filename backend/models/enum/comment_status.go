package enum

type CommentStatus int8

const (
	CommentStatusExamining CommentStatus = iota + 1
	CommentStatusPublished
	CommentStatusDeleted
)
