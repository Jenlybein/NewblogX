package models

import (
	_ "embed"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"time"
)

// ESTag 是文章写入 ES 时使用的标签结构。
// 这里保留标签 id 和 title，方便 ES 侧按单字段过滤，也方便列表展示。
type ESTag struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

// ArticleModel 文章表
type ArticleModel struct {
	Model
	Title            string                       `gorm:"size:256" json:"title"`
	Abstract         string                       `gorm:"size:256" json:"abstract"`
	Content          string                       `gorm:"type:longtext" json:"content"`
	ContentHead      string                       `gorm:"-" json:"content_head,omitempty"` // ES 冗余字段，保存去除 Markdown 格式后的正文前 150 字
	CategoryID       *ctype.ID                    `gorm:"index" json:"category_id"`
	Cover            string                       `gorm:"size:256" json:"cover"`
	AuthorID         ctype.ID                     `gorm:"index" json:"author_id"`
	ViewCount        int                          `gorm:"default:0" json:"view_count"`
	DiggCount        int                          `gorm:"default:0" json:"digg_count"`
	CommentCount     int                          `gorm:"default:0" json:"comment_count"`
	FavorCount       int                          `gorm:"default:0" json:"favor_count"`
	CommentsToggle   bool                         `gorm:"default:true" json:"comments_toggle"`
	PublishStatus    enum.ArticleStatus           `gorm:"default:0;index" json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `gorm:"type:varchar(32);default:'visible';index" json:"visibility_status"`
	SubmittedAt      *time.Time                   `json:"submitted_at"`
	ReviewedAt       *time.Time                   `json:"reviewed_at"`
	ReviewedBy       *ctype.ID                    `gorm:"index" json:"reviewed_by"`
	UserModel        UserModel                    `gorm:"foreignKey:AuthorID;references:ID" json:"-"`
	CategoryModel    *CategoryModel               `gorm:"foreignKey:CategoryID;references:ID" json:"-"`
	Tags             []TagModel                   `gorm:"many2many:article_tag_models;joinForeignKey:ArticleID;joinReferences:TagID" json:"tags"`
}

func (a ArticleModel) EffectivePublishStatus() enum.ArticleStatus {
	return a.PublishStatus
}

func (a ArticleModel) EffectiveVisibilityStatus() enum.ArticleVisibilityStatus {
	if a.VisibilityStatus == "" {
		return enum.ArticleVisibilityVisible
	}
	return a.VisibilityStatus
}

func (a ArticleModel) IsPublicVisible() bool {
	return a.EffectivePublishStatus() == enum.ArticleStatusPublished &&
		a.EffectiveVisibilityStatus() == enum.ArticleVisibilityVisible
}

//go:embed es_settings/article_mapping.json
var ArticleMapping string

func (ArticleModel) Mapping() string {
	return ArticleMapping
}

func (ArticleModel) Index() string {
	return DefaultArticleESIndex
}

//go:embed es_settings/article_pipeline.json
var ArticlePipeline string

func (ArticleModel) Pipeline() string {
	return ArticlePipeline
}

func (ArticleModel) PipelineName() string {
	return "article_pipeline"
}
