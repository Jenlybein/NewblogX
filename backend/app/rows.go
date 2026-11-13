package app

import (
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
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
	// 质量闸门的异步判定（ADR-0030）只认「已发布待可见」这一档，所以要状态与可见性。
	PublishStatus int    `gorm:"column:publish_status"`
	Visibility    string `gorm:"column:visibility_status"`
}

// TableName 固定表名。
func (noticeArticleRow) TableName() string { return "article_models" }

// noticeMessageRow 只用来做"通知是否已存在"的 count，不含读取列。
type noticeMessageRow struct{}

// TableName 固定表名。
func (noticeMessageRow) TableName() string { return "article_message_models" }

// noticeReviewReasonRow 是发驳回通知时读的审核任务信息（只取驳回原因）。
//
// 只查一列：装配层不需要审核任务的其余字段，多查一列就多一处随表结构漂移的耦合。
type noticeReviewReasonRow struct {
	Reason string
}

// TableName 固定表名。
func (noticeReviewReasonRow) TableName() string { return "article_review_task_models" }

// tagTitleRow 是"标签 id → 标题"的映射行。
type tagTitleRow struct {
	ID    ctype.ID
	Title string
}

// TableName 固定表名。
func (tagTitleRow) TableName() string { return "tag_models" }

// noticeUserRoleRow 是"用户 id → 角色"的映射行（只取角色）。
//
// 异步链路没有登录态，但 AI 每日额度的管理员豁免与调用日志的归属都需要它；
// 只查一列，不引入整行用户结构（那会随表结构漂移）。
type noticeUserRoleRow struct {
	Role enum.RoleType
}

// TableName 固定表名。
func (noticeUserRoleRow) TableName() string { return "user_models" }
