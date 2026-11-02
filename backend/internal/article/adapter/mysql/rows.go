package mysql

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// 本文件把 article 模块用到的表行结构复制进来。
//
// 为什么复制：架构守卫（R10）要求业务模块不 import models。
// row 结构（GORM 标签、列名、TableName）是"这张表长什么样"的唯一真相，
// 它应该跟着模块走；models 只剩"还没迁走的模块"在用。
//
// 约定：字段的 gorm 标签、默认值、索引名必须与 models 里逐字一致，
// 否则 GORM 生成的 SQL 会指向不存在的列，而且**编译不会报错**。

// BaseRow 是所有行的公共字段。
//
// 必须导出：GORM 用反射给字段赋值，未导出的内嵌结构体它写不进去，
// 结果是主键和时间列静默变成零值（集成测试会立刻发现）。
//
// BeforeCreate 必须保留：主键是 Snowflake，由代码生成，
// 数据库没有默认值——不走这个钩子，Create 会报 "Field 'id' doesn't have a default value"。
type BaseRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// BeforeCreate 生成雪花主键。
func (b *BaseRow) BeforeCreate(tx *gorm.DB) error {
	if !b.ID.IsZero() {
		return nil
	}
	next, err := idgen.Next()
	if err != nil {
		return err
	}
	b.ID = next
	return nil
}

// 发布状态：与 models/enum 一致（1 草稿 / 2 审核中 / 3 已发布 / 4 已拒绝）。
const (
	statusDraft     int8 = 1
	statusExamining int8 = 2
	statusPublished int8 = 3
	statusRejected  int8 = 4
)

// 可见性：与 models/enum 一致。
const (
	visibilityVisible     = "visible"
	visibilityUserHidden  = "user_hidden"
	visibilityAdminHidden = "admin_hidden"
)

// articleRow 对应 article_models 表。
type articleRow struct {
	BaseRow
	Title            string
	Abstract         string
	Content          string `gorm:"type:longtext"`
	CategoryID       *ctype.ID
	Cover            string
	AuthorID         ctype.ID
	ViewCount        int
	DiggCount        int
	CommentCount     int
	FavorCount       int
	CommentsToggle   bool
	PublishStatus    int8   `gorm:"type:tinyint"`
	VisibilityStatus string `gorm:"type:varchar(32)"`
	SubmittedAt      *time.Time
	ReviewedAt       *time.Time
	ReviewedBy       *ctype.ID
}

// TableName 固定表名。
func (articleRow) TableName() string { return "article_models" }

// articleTagRow 对应 article_tag_models（many2many 纯 join 表）。
type articleTagRow struct {
	BaseRow
	ArticleID ctype.ID
	TagID     ctype.ID
}

// TableName 固定表名。
func (articleTagRow) TableName() string { return "article_tag_models" }

// articleDiggRow 对应 article_digg_models。
type articleDiggRow struct {
	BaseRow
	ArticleID ctype.ID
	UserID    ctype.ID
}

// TableName 固定表名。
func (articleDiggRow) TableName() string { return "article_digg_models" }

// commentRow 对应 comment_models（本模块只统计/删除，不读内容）。
type commentRow struct {
	BaseRow
	ArticleID ctype.ID
}

// TableName 固定表名。
func (commentRow) TableName() string { return "comment_models" }

// userRow 对应 user_models（本模块只读作者展示信息）。
type userRow struct {
	BaseRow
	Username string
	Nickname string
	Avatar   string
	Abstract string
	// Role 是用户角色（1 管理员 / 2 普通 / 3 受限访客）。
	//
	// 只读：读面过滤要判断"作者是不是管理员"（ADR-0017 决策 4），
	// 这是 article 模块唯一需要知道的用户角色信息。
	Role int8 `gorm:"type:tinyint"`
}

// TableName 固定表名。
func (userRow) TableName() string { return "user_models" }

// userStatRow 对应 user_stat_models（只读展示需要的三项累计值）。
//
// 为什么由 article 模块读 user 的表：作者卡属于文章页的展示需求，
// 为了三个计数再走一次跨模块调用不值得。这是**只读**的窄行结构，
// 与 userRow 读 user_models 是同一套做法；写侧仍然只走 user 模块。
type userStatRow struct {
	UserID              ctype.ID `gorm:"column:user_id;primaryKey"`
	ArticleCount        int      `gorm:"column:article_count"`
	ArticleVisitedCount int      `gorm:"column:article_visited_count"`
	FansCount           int      `gorm:"column:fans_count"`
}

// TableName 固定表名。
func (userStatRow) TableName() string { return "user_stat_models" }

// favoriteRow 对应 favorite_models。
type favoriteRow struct {
	BaseRow
	UserID        ctype.ID
	Title         string
	Cover         string
	Abstract      string
	IsDefault     bool
	ArticleCount  int
	OwnerNickname string
	OwnerAvatar   string
}

// TableName 固定表名。
func (favoriteRow) TableName() string { return "favorite_models" }

// userArticleFavorRow 对应 user_article_favor_models。
type userArticleFavorRow struct {
	BaseRow
	ArticleID             ctype.ID
	UserID                ctype.ID
	FavorID               ctype.ID
	ArticleTitle          string
	ArticleAbstract       string
	ArticleCover          string
	ArticleStatus         int8 `gorm:"type:tinyint"`
	ArticleAuthorID       ctype.ID
	ArticleAuthorNickname string
	ArticleAuthorAvatar   string
}

// TableName 固定表名。
func (userArticleFavorRow) TableName() string { return "user_article_favor_models" }

// userTopArticleRow 对应 user_top_article_models。
type userTopArticleRow struct {
	BaseRow
	UserID    ctype.ID
	ArticleID ctype.ID
}

// TableName 固定表名。
func (userTopArticleRow) TableName() string { return "user_top_article_models" }

// articleHotTopRow 对应 article_hot_top_models（热门置顶运营位）。
//
// 与 userTopArticleRow 长得像但**不是同一件事**：那张是作者置顶自己的文章
// （个人主页），这张是管理员运营的首页位（ADR-0017 决策 5）。
// article_id 唯一：一篇文章最多出现在运营位一次。
type articleHotTopRow struct {
	BaseRow
	ArticleID ctype.ID `gorm:"uniqueIndex:uk_article_hot_top"`
	SortOrder int      `gorm:"not null;default:0"`
}

// TableName 固定表名。
func (articleHotTopRow) TableName() string { return "article_hot_top_models" }

// userArticleViewHistoryRow 对应 user_article_view_history_models。
type userArticleViewHistoryRow struct {
	BaseRow
	ArticleID ctype.ID
	UserID    ctype.ID
}

// TableName 固定表名。
func (userArticleViewHistoryRow) TableName() string { return "user_article_view_history_models" }

// 审核任务的状态/阶段/来源：与 models 一致。
const (
	reviewTaskPending  = "pending"
	reviewTaskApproved = "approved"
	reviewTaskRejected = "rejected"
	reviewTaskCanceled = "canceled"

	reviewTaskStageManual = "manual"

	reviewTaskSourceCreate   = "create"
	reviewTaskSourceEdit     = "edit"
	reviewTaskSourceResubmit = "resubmit"
)

// reviewTaskRow 对应 article_review_task_models。
type reviewTaskRow struct {
	BaseRow
	ArticleID            ctype.ID
	AuthorID             ctype.ID
	ArticleTitle         string
	AuthorName           string
	ArticlePublishStatus int8   `gorm:"type:tinyint"`
	Stage                string `gorm:"type:varchar(16)"`
	Source               string `gorm:"type:varchar(16)"`
	Status               string `gorm:"type:varchar(16)"`
	Reason               string
	ReviewedAt           *time.Time
	ReviewedBy           *ctype.ID
}

// TableName 固定表名。
func (reviewTaskRow) TableName() string { return "article_review_task_models" }

// categoryRow 对应 category_models。
type categoryRow struct {
	BaseRow
	Title  string
	UserID ctype.ID
}

// TableName 固定表名。
func (categoryRow) TableName() string { return "category_models" }

// tagRow 对应 tag_models。
type tagRow struct {
	BaseRow
	Title        string
	Sort         int
	Description  string
	ArticleCount int
	IsEnabled    bool
	CreatedBy    ctype.ID
}

// TableName 固定表名。
func (tagRow) TableName() string { return "tag_models" }

// outboxRow 对应 outbox_event_models。
type outboxRow struct {
	BaseRow
	EventName     string
	AggregateType string
	AggregateID   ctype.ID
	Payload       string `gorm:"type:longtext"`
	Status        string `gorm:"size:16"`
	Attempts      int
	AvailableAt   time.Time
	LastError     string
	DispatchedAt  *time.Time
}

// TableName 固定表名。
func (outboxRow) TableName() string { return "outbox_event_models" }

// outboxStatusPending 是 outbox 事件的初始状态。
const outboxStatusPending = "pending"

// collectionRow 对应 collection_models 表（合集主表）。
//
// 字段与 testkit/schema 的 CollectionRow 逐字一致：schema_sql_integration_test
// 会把 schema.sql 建出来的表与 testkit 的行结构逐列比对，差一个 tag 就红。
type collectionRow struct {
	BaseRow
	Title       string   `gorm:"type:varchar(128);not null"`
	Description string   `gorm:"type:varchar(512);not null"`
	Cover       string   `gorm:"type:longtext;not null"`
	IsDisplay   bool     `gorm:"not null;default:1"`
	SortOrder   int      `gorm:"not null;default:0"`
	CreatorID   ctype.ID `gorm:"not null"`
}

// TableName 固定表名。
func (collectionRow) TableName() string { return "collection_models" }

// collectionArticleRow 对应 collection_article_models 表（合集-文章中间表）。
//
// 唯一索引 uk_collection_article **不含 deleted_at**：移除是软删，行留在表里
// 占着这个键，所以"移除后再收录"必须恢复原行而不是新插入（与点赞表
// uk_article_digg 是同一个坑）。
type collectionArticleRow struct {
	BaseRow
	CollectionID ctype.ID `gorm:"uniqueIndex:uk_collection_article,priority:1;not null"`
	ArticleID    ctype.ID `gorm:"uniqueIndex:uk_collection_article,priority:2;not null;index:idx_collection_article_models_article_id"`
	SortOrder    int      `gorm:"not null;default:0"`
}

// TableName 固定表名。
func (collectionArticleRow) TableName() string { return "collection_article_models" }

