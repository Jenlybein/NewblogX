package enum

type ArticleStatus int8

const (
	ArticleStatusDraft     ArticleStatus = iota + 1 // 草稿
	ArticleStatusExamining                          // 审核中
	ArticleStatusPublished                          // 已发布
	ArticleStatusRejected                           // 已拒绝
)
