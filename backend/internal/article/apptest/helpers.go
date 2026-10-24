package apptest

import (
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/article/domain/top"
)

// 本文件是"少写点样板"的构造助手。
// 它们只做字段填充，不含业务规则——规则一律走 domain 的构造函数。

// NewCategoryForTest 构造分类（含指定用户）。
func NewCategoryForTest(id domain.CategoryID, userID domain.UserID, title string) *taxonomy.Category {
	return taxonomy.RestoreCategory(id, userID, title)
}

// NewTagForTest 构造标签，可指定是否启用。
func NewTagForTest(id domain.TagID, title string, enabled bool) *taxonomy.Tag {
	return taxonomy.RestoreTag(taxonomy.RestoreTagParams{
		ID:      id,
		Title:   title,
		Enabled: enabled,
	})
}

// NewTaskForTest 构造审核任务。
func NewTaskForTest(id uint64, articleID domain.ArticleID, authorID domain.UserID, status review.Status) *review.Task {
	return review.RestoreTask(review.RestoreTaskParams{
		ID:        id,
		ArticleID: articleID,
		AuthorID:  authorID,
		Stage:     review.StageManual,
		Source:    review.SourceCreate,
		Status:    status,
		CreatedAt: DefaultTime,
	})
}

// NewTopEntryForTest 构造置顶关系。
func NewTopEntryForTest(userID domain.UserID, articleID domain.ArticleID) *top.Entry {
	return top.NewEntry(userID, articleID)
}
