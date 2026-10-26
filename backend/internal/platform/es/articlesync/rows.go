package articlesync

import (
	_ "embed"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/mysql"
)

// DefaultArticleESIndex 是文章索引的默认名。
//
// 从 models/es_index.go 搬来：索引名是 ES 投影的知识，不是"数据模型"。
const DefaultArticleESIndex = "article_index"

// ESTag 是 ES 文档里的标签项（只有 id 与标题）。
type ESTag struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

// ArticleRow 是 article_models 的完整行（含构建文档要用的关联）。
//
// 为什么本包自己持有一份而不是复用某个共享 models：ES 投影是本包唯一的知识，
// 它读哪些列、怎么拼文档，都只在这里成立。代价是同一张表在别处还有一份定义
// —— 两份都必须与真表对得上，这一点由 testkit/schema 的列名契约测试兜住。
type ArticleRow struct {
	mysql.BaseRow

	Title            string                       `gorm:"size:256"`
	Abstract         string                       `gorm:"size:256"`
	Content          string                       `gorm:"type:longtext"`
	ContentHead      string                       `gorm:"-"` // ES 冗余字段，不落库
	CategoryID       *ctype.ID                    `gorm:"index"`
	Cover            string                       `gorm:"size:256"`
	AuthorID         ctype.ID                     `gorm:"index"`
	ViewCount        int                          `gorm:"default:0"`
	DiggCount        int                          `gorm:"default:0"`
	CommentCount     int                          `gorm:"default:0"`
	FavorCount       int                          `gorm:"default:0"`
	CommentsToggle   bool                         `gorm:"default:true"`
	PublishStatus    enum.ArticleStatus           `gorm:"default:0;index"`
	VisibilityStatus enum.ArticleVisibilityStatus `gorm:"type:varchar(32);default:'visible';index"`
	SubmittedAt      *time.Time
	ReviewedAt       *time.Time
	ReviewedBy       *ctype.ID `gorm:"index"`

	UserModel     UserRow      `gorm:"foreignKey:AuthorID;references:ID"`
	CategoryModel *CategoryRow `gorm:"foreignKey:CategoryID;references:ID"`
	Tags          []TagRow     `gorm:"many2many:article_tag_models;joinForeignKey:ArticleID;joinReferences:TagID"`
}

// TableName 固定表名。
func (ArticleRow) TableName() string { return "article_models" }

// Index 返回该文档所属的 ES 索引名。
func (ArticleRow) Index() string { return DefaultArticleESIndex }

// 索引的 mapping 与 ingest pipeline 跟着投影走：
// 它们是"文档长什么样"的一部分，不属于任何数据模型。
//
//go:embed mapping/article_mapping.json
var articleMapping string

//go:embed mapping/article_pipeline.json
var articlePipeline string

// Mapping 返回索引定义。
func (ArticleRow) Mapping() string { return articleMapping }

// Pipeline 返回 ingest pipeline 定义。
func (ArticleRow) Pipeline() string { return articlePipeline }

// PipelineName 返回 ingest pipeline 名字。
func (ArticleRow) PipelineName() string { return "article_pipeline" }

// TagRow 是 tag_models 的行。
type TagRow struct {
	mysql.BaseRow

	Title        string   `gorm:"size:64;uniqueIndex"`
	Sort         int      `gorm:"default:0"`
	Description  string   `gorm:"size:255"`
	ArticleCount int      `gorm:"default:0"`
	IsEnabled    bool     `gorm:"default:true;index"`
	CreatedBy    ctype.ID `gorm:"index"`
}

// TableName 固定表名。
func (TagRow) TableName() string { return "tag_models" }

// CategoryRow 是 category_models 的行。
type CategoryRow struct {
	mysql.BaseRow

	Title  string   `gorm:"size:256"`
	UserID ctype.ID `gorm:"index"`
}

// TableName 固定表名。
func (CategoryRow) TableName() string { return "category_models" }

// UserRow 是 user_models 的行（ES 文档只用到展示字段，但它们必须齐全）。
type UserRow struct {
	mysql.BaseRow

	Username              string                  `gorm:"size:32;uniqueIndex"`
	Nickname              string                  `gorm:"size:32"`
	Avatar                string                  `gorm:"size:256"`
	Abstract              string                  `gorm:"size:256"`
	RegisterSource        enum.RegisterSourceType `gorm:""`
	Password              string                  `gorm:"size:64"`
	Email                 *string                 `gorm:"size:256;uniqueIndex"`
	OpenID                *string                 `gorm:"size:64;uniqueIndex"`
	Status                enum.UserStatus         `gorm:"default:1;index"`
	TokenVersion          uint32                  `gorm:"default:1"`
	LastPasswordChangedAt *time.Time
	Role                  enum.RoleType `gorm:"default:0"`
	IP                    string        `gorm:"size:64"`
	Addr                  string        `gorm:"size:256"`
}

// TableName 固定表名。
func (UserRow) TableName() string { return "user_models" }
// ArticleTagRow 是文章与标签的关系行。
type ArticleTagRow struct {
	mysql.BaseRow

	ArticleID ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:1;index:idx_article_tag_article,priority:1"`
	TagID     ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:2;index:idx_article_tag_tag,priority:1"`
}

// TableName 固定表名。
func (ArticleTagRow) TableName() string { return "article_tag_models" }

// UserTopArticleRow 是用户置顶行。
type UserTopArticleRow struct {
	mysql.BaseRow

	UserID    ctype.ID `gorm:"uniqueIndex:uk_user_top_article,priority:1"`
	ArticleID ctype.ID `gorm:"uniqueIndex:uk_user_top_article,priority:2"`
}

// TableName 固定表名。
func (UserTopArticleRow) TableName() string { return "user_top_article_models" }
