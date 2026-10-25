package app

import (
	"myblogx/internal/kernel/ctype"
)

// 本文件是装配层的**查询投影**。
//
// 装配层不是一个 adapter，它跨模块读几个字段把服务接起来（发通知、校验标签）。
// 这些读取都是"只取几列"，所以用最小投影而不是整行结构：
//
//   - 显式列写在这里，写错了 SELECT 会直接报 Unknown column；
//   - 整行结构才需要"漏字段"的那种担心（少一列 GORM 不写它，读到上一版旧值），
//     见 internal/testkit/schema 的 ColumnsDiff 说明。
//
// 三个结构都不含主键时间列：没有写操作，就不需要 BeforeCreate 那套约定。

// noticeArticleRow 是发通知时读的文章信息（标题 + 作者）。
type noticeArticleRow struct {
	ID       ctype.ID
	Title    string
	AuthorID ctype.ID
}

// TableName 固定表名。
func (noticeArticleRow) TableName() string { return "article_models" }

// noticeMessageRow 只用来做"通知是否已存在"的 count，不含读取列。
type noticeMessageRow struct{}

// TableName 固定表名。
func (noticeMessageRow) TableName() string { return "article_message_models" }

// tagTitleRow 是"标签 id → 标题"的映射行。
type tagTitleRow struct {
	ID    ctype.ID
	Title string
}

// TableName 固定表名。
func (tagTitleRow) TableName() string { return "tag_models" }
