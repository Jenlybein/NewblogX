package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/risk"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"
)

// articleRiskRow 对应表 article_risk_models（票 312 / ADR-0033）。
//
// 这是**新表**：文章表与审核任务表都不加列（零 DDL 硬约束）。一篇文章一行，
// 唯一键在 article_id 上，随每次判定覆盖写。
type articleRiskRow struct {
	ID             ctype.ID   `gorm:"primaryKey;column:id"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at;index"`
	ArticleID      ctype.ID   `gorm:"column:article_id;uniqueIndex:uk_article_risk_article"`
	Verdict        string     `gorm:"column:verdict"`
	RiskLevel      int        `gorm:"column:risk_level"`
	CategoriesJSON string     `gorm:"column:categories_json"`
	ReasonsJSON    string     `gorm:"column:reasons_json"`
	PromptVersion  string     `gorm:"column:prompt_version"`
	ModelName      string     `gorm:"column:model_name"`
	CheckedAt      *time.Time `gorm:"column:checked_at"`
}

// TableName 返回表名。
func (articleRiskRow) TableName() string { return "article_risk_models" }

// RiskRepo 是 risk.Repository 的 MySQL 实现。
type RiskRepo struct {
	db *gorm.DB
}

// NewRiskRepo 构造风险记录仓储。
func NewRiskRepo(db *gorm.DB) *RiskRepo { return &RiskRepo{db: db} }

// Upsert 写入或覆盖一篇文章的结论。
//
// 走"先查后写"而不是 ON DUPLICATE KEY：唯一键在 article_id 上，覆盖时要保留
// 原主键（同一篇文章的记录 ID 稳定），而 ON DUPLICATE KEY 在 MySQL 里只保证
// 行不被重复插入，主键归属要靠 LAST_INSERT_ID 取回——这层隐式语义不比显式
// 查询更省事。
func (r *RiskRepo) Upsert(ctx context.Context, record *risk.Record) error {
	if record == nil || record.ArticleID.IsZero() {
		return apperr.New(apperr.CodeArticleNotFound)
	}
	now := time.Now()

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing articleRiskRow
		err := tx.Where("article_id = ?", record.ArticleID).Take(&existing).Error
		switch {
		case err == nil:
			checkedAt := record.CheckedAt
			if checkedAt.IsZero() {
				checkedAt = now
			}
			return tx.Model(&articleRiskRow{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"updated_at":      now,
					"verdict":         string(record.Verdict),
					"risk_level":      record.RiskLevel,
					"categories_json": record.CategoriesJSON,
					"reasons_json":    record.ReasonsJSON,
					"prompt_version":  record.PromptVersion,
					"model_name":      record.ModelName,
					"checked_at":      checkedAt,
				}).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			id, idErr := idgen.Next()
			if idErr != nil {
				return idErr
			}
			checkedAt := record.CheckedAt
			if checkedAt.IsZero() {
				checkedAt = now
			}
			return tx.Create(&articleRiskRow{
				ID:             id,
				CreatedAt:      now,
				UpdatedAt:      now,
				ArticleID:      record.ArticleID,
				Verdict:        string(record.Verdict),
				RiskLevel:      record.RiskLevel,
				CategoriesJSON: record.CategoriesJSON,
				ReasonsJSON:    record.ReasonsJSON,
				PromptVersion:  record.PromptVersion,
				ModelName:      record.ModelName,
				CheckedAt:      &checkedAt,
			}).Error
		default:
			return err
		}
	})
}

// FindByArticle 读一篇文章的结论；没有记录时返回 (nil, nil)。
func (r *RiskRepo) FindByArticle(ctx context.Context, articleID domain.ArticleID) (*risk.Record, error) {
	var row articleRiskRow
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	record := toRiskRecord(row)
	return &record, nil
}

// ListByArticles 批量读结论，键为文章 ID。
func (r *RiskRepo) ListByArticles(ctx context.Context, articleIDs []domain.ArticleID) (map[ctype.ID]*risk.Record, error) {
	out := make(map[ctype.ID]*risk.Record, len(articleIDs))
	if len(articleIDs) == 0 {
		return out, nil
	}
	var rows []articleRiskRow
	if err := r.db.WithContext(ctx).Where("article_id IN ?", articleIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := toRiskRecord(row)
		out[row.ArticleID] = &record
	}
	return out, nil
}

// toRiskRecord 把 row 转成领域记录。
func toRiskRecord(row articleRiskRow) risk.Record {
	checkedAt := time.Time{}
	if row.CheckedAt != nil {
		checkedAt = *row.CheckedAt
	}
	return risk.Record{
		ArticleID:      row.ArticleID,
		Verdict:        risk.Normalize(row.Verdict),
		RiskLevel:      row.RiskLevel,
		CategoriesJSON: row.CategoriesJSON,
		ReasonsJSON:    row.ReasonsJSON,
		PromptVersion:  row.PromptVersion,
		ModelName:      row.ModelName,
		CheckedAt:      checkedAt,
	}
}
