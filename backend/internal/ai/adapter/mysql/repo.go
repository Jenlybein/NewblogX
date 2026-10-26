// Package mysql 是 AI 模块的 MySQL 适配器。
package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"myblogx/internal/kernel/ctype"
)

// scoreRow 对应 article_ai_score_record_models 表。
type scoreRow struct {
	ID                       ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt
	ArticleID                ctype.ID
	UserID                   ctype.ID
	TitleSnapshot            string
	ContentHash              string
	ContentLength            int
	ArticleUpdatedAtSnapshot *time.Time
	AITotalScore             int
	TotalScore               int
	ScoreLevel               string
	ArticleType              string
	DimensionsJSON           string
	MainIssuesJSON           string
	OverallComment           string
	Provider                 string
	ModelName                string
	PromptVersion            string
}

// TableName 固定表名。
func (scoreRow) TableName() string { return "article_ai_score_record_models" }

// articleRow 是评分需要读的文章片段。
type articleRow struct {
	ID            ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	AuthorID      ctype.ID
	Title         string
	Content       string
	UpdatedAt     time.Time
	PublishStatus int8
}

// TableName 固定表名。
func (articleRow) TableName() string { return "article_models" }

// 发布状态：与 models/enum.ArticleStatusPublished 保持一致（业务模块不 import models）。
const publishStatusPublished int8 = 3

// Repo 实现评分持久化端口。
type Repo struct {
	db *gorm.DB
}

var _ domain.ScoreRepository = (*Repo)(nil)

// New 构造评分仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// Article 实现 domain.ScoreRepository。
func (r *Repo) Article(ctx context.Context, id uint64) (domain.ArticleSnapshot, error) {
	var row articleRow
	err := r.db.WithContext(ctx).
		Select("id", "author_id", "title", "content", "updated_at", "publish_status").
		Where("id = ?", id).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ArticleSnapshot{}, apperr.New(apperr.CodeAIArticleMissing)
		}
		return domain.ArticleSnapshot{}, err
	}
	return domain.ArticleSnapshot{
		ID:        row.ID,
		AuthorID:  row.AuthorID,
		Title:     row.Title,
		Content:   row.Content,
		UpdatedAt: row.UpdatedAt,
		Published: row.PublishStatus == publishStatusPublished,
	}, nil
}

// Latest 实现 domain.ScoreRepository。
func (r *Repo) Latest(ctx context.Context, articleID uint64) (*domain.ScoreRecord, error) {
	var row scoreRow
	err := r.db.WithContext(ctx).
		Where("article_id = ?", articleID).
		Order("created_at desc").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	record := toRecord(row)
	return &record, nil
}

// CreateIfAbsent 实现 domain.ScoreRepository。
func (r *Repo) CreateIfAbsent(ctx context.Context, record *domain.ScoreRecord) (bool, error) {
	if record == nil {
		return false, apperr.New(apperr.CodeAIScoreSaveFailed)
	}
	if record.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return false, err
		}
		record.ID = next
	}
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&scoreRow{}).
			Where("article_id = ?", record.ArticleID).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		now := time.Now()
		row := scoreRow{
			ID:                       record.ID,
			CreatedAt:                now,
			UpdatedAt:                now,
			ArticleID:                record.ArticleID,
			UserID:                   record.UserID,
			TitleSnapshot:            record.TitleSnapshot,
			ContentHash:              record.ContentHash,
			ContentLength:            record.ContentLength,
			ArticleUpdatedAtSnapshot: record.ArticleUpdatedAt,
			AITotalScore:             record.AITotalScore,
			TotalScore:               record.TotalScore,
			ScoreLevel:               record.ScoreLevel,
			ArticleType:              record.ArticleType,
			DimensionsJSON:           record.DimensionsJSON,
			MainIssuesJSON:           record.MainIssuesJSON,
			OverallComment:           record.OverallComment,
			Provider:                 record.Provider,
			ModelName:                record.ModelName,
			PromptVersion:            record.PromptVersion,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		record.CreatedAt = now
		created = true
		return nil
	})
	return created, err
}

// toRecord 把表行转成领域实体。
func toRecord(row scoreRow) domain.ScoreRecord {
	return domain.ScoreRecord{
		ID:               row.ID,
		ArticleID:        row.ArticleID,
		UserID:           row.UserID,
		TitleSnapshot:    row.TitleSnapshot,
		ContentHash:      row.ContentHash,
		ContentLength:    row.ContentLength,
		ArticleUpdatedAt: row.ArticleUpdatedAtSnapshot,
		AITotalScore:     row.AITotalScore,
		TotalScore:       row.TotalScore,
		ScoreLevel:       row.ScoreLevel,
		ArticleType:      row.ArticleType,
		DimensionsJSON:   row.DimensionsJSON,
		MainIssuesJSON:   row.MainIssuesJSON,
		OverallComment:   row.OverallComment,
		Provider:         row.Provider,
		ModelName:        row.ModelName,
		PromptVersion:    row.PromptVersion,
		CreatedAt:        row.CreatedAt,
	}
}

// Catalog 提供分类与标签候选。
type Catalog struct {
	db *gorm.DB
}

var _ domain.Catalog = (*Catalog)(nil)

// NewCatalog 构造候选读取器。
func NewCatalog(db *gorm.DB) *Catalog { return &Catalog{db: db} }

// Categories 实现 domain.Catalog。
func (c *Catalog) Categories(ctx context.Context, userID uint64) ([]domain.Option, error) {
	return c.options(ctx, "category_models", "user_id = ?", userID)
}

// Tags 实现 domain.Catalog。
func (c *Catalog) Tags(ctx context.Context) ([]domain.Option, error) {
	return c.options(ctx, "tag_models", "is_enabled = ?", true)
}

// options 是两类候选的共同查询。
func (c *Catalog) options(ctx context.Context, table, where string, arg any) ([]domain.Option, error) {
	var rows []struct {
		ID    ctype.ID
		Title string
	}
	order := "id asc"
	if table == "tag_models" {
		order = "sort desc, id asc"
	}
	err := c.db.WithContext(ctx).
		Table(table).
		Select("id", "title").
		Where(where+" AND deleted_at IS NULL", arg).
		Order(order).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Option, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Option{ID: row.ID, Title: row.Title})
	}
	return out, nil
}

// TagTitles 实现 domain.TagReader。
func (c *Catalog) TagTitles(ctx context.Context) ([]string, error) {
	var titles []string
	err := c.db.WithContext(ctx).
		Table("tag_models").
		Where("is_enabled = ? AND deleted_at IS NULL", true).
		Order("sort desc, id asc").
		Pluck("title", &titles).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(titles))
	for _, title := range titles {
		if strings.TrimSpace(title) == "" {
			continue
		}
		out = append(out, title)
	}
	return out, nil
}
