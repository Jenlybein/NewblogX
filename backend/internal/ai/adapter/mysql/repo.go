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
	"gorm.io/gorm/clause"
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

// Upsert 实现 domain.ScoreRepository：同一篇文章写一行，已有则整体覆盖。
//
// # 为什么不是"有记录就跳过"
//
// 旧实现发现该文章已有评分就什么都不写，于是"被驳回 → 改稿 → 再评分"这条路上，
// 作者永远看到第一次算出来的分数。评分结论必须可重算、可覆盖。
//
// # 为什么这样写才只有一行
//
// 表上 article_id 没有唯一索引（零 DDL 加不了），所以"只插一次"不能靠数据库约束兜底：
// 两个并发的评分请求都会读到"还没有行"，然后各插一行。这里退回到最朴素的排他手段 ——
// 事务第一步先锁住这篇文章在 article_models 里的那一行，拿到锁之后才判断有没有评分行。
// 同一篇文章的并发评分因此在文章行上排队，判断与写入之间插不进别的写者。
//
// 覆盖写保留原行的主键（对外的 record_id 不会跳），但把 created_at 一起推到本次评分时间：
// 这一行代表"最近一次评分"而不是评分历史，读者页与作者页都把它显示成评分时间，
// 留着首次时间会让人以为没重算过。
func (r *Repo) Upsert(ctx context.Context, record *domain.ScoreRecord) error {
	if record == nil {
		return apperr.New(apperr.CodeAIScoreSaveFailed)
	}
	if record.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		record.ID = next
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockArticle(tx, record.ArticleID); err != nil {
			return err
		}
		now := time.Now()
		// Unscoped：软删过的旧记录也要被覆盖回来，否则会变成"一行旧的 + 一行新的"。
		var existing scoreRow
		query := tx.Unscoped().
			Where("article_id = ?", record.ArticleID).
			Order("id asc").
			Limit(1).
			Find(&existing)
		if query.Error != nil {
			return query.Error
		}

		// 用 map 而不是结构体：结构体会被 GORM 跳过零值（分数或篇幅真的可以是 0）。
		columns := map[string]any{
			"created_at":                  now,
			"updated_at":                  now,
			"deleted_at":                  nil,
			"article_id":                  record.ArticleID,
			"user_id":                     record.UserID,
			"title_snapshot":              record.TitleSnapshot,
			"content_hash":                record.ContentHash,
			"content_length":              record.ContentLength,
			"article_updated_at_snapshot": record.ArticleUpdatedAt,
			"ai_total_score":              record.AITotalScore,
			"total_score":                 record.TotalScore,
			"score_level":                 record.ScoreLevel,
			"article_type":                record.ArticleType,
			"dimensions_json":             record.DimensionsJSON,
			"main_issues_json":            record.MainIssuesJSON,
			"overall_comment":             record.OverallComment,
			"provider":                    record.Provider,
			"model_name":                  record.ModelName,
			"prompt_version":              record.PromptVersion,
		}

		record.CreatedAt = now
		if query.RowsAffected == 0 {
			columns["id"] = record.ID
			if err := tx.Model(&scoreRow{}).Create(columns).Error; err != nil {
				return err
			}
			return nil
		}
		record.ID = existing.ID
		return tx.Unscoped().Model(&scoreRow{}).Where("id = ?", existing.ID).Updates(columns).Error
	})
}

// lockArticle 给这篇文章在 article_models 里的那行加排他锁，让同一篇文章的并发评分排队。
//
// MySQL 用 SELECT ... FOR UPDATE。SQLite（仓储测试库）没有行锁语法，
// 直接跳过：它本身就是单写者，而且 testkit 的连接池只开一条连接。
func lockArticle(tx *gorm.DB, articleID ctype.ID) error {
	if tx.Dialector.Name() != "mysql" {
		return nil
	}
	var row articleRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		Where("id = ?", articleID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(apperr.CodeAIArticleMissing)
	}
	return err
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
