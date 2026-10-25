package message_enum

type Type int8

const (
	CommentArticleType Type = iota + 1
	CommentReplyType
	DiggArticleType
	UnDiggArticleType
	DiggCommentType
	UnDiggCommentType
	FavorArticleType
	UnFavorArticleType
	SystemType
)

type BizType int8

const (
	ArticleType BizType = iota + 1
	CommentType
)
