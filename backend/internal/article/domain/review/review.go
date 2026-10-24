// Package review 是文章审核任务子域。
//
// 审核任务是一条独立记录（不是文章状态本身）：文章进审核时创建任务，
// 审核通过/拒绝/撤销时改变任务状态。这样"审核队列"可以按任务查，
// 也能保留每次提交的历史，而不是只看到文章当前状态。
package review

import (
	"context"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// 子域错误。文案统一在 internal/apperr 的消息表里。
var (
	// ErrTaskNotFound 审核任务不存在。
	ErrTaskNotFound = apperr.New(apperr.CodeReviewTaskNotFound)
	// ErrTaskHandled 任务已被处理，不能重复处理。
	ErrTaskHandled = apperr.New(apperr.CodeReviewTaskHandled)
	// ErrTaskForbidden 无权处理该任务。
	ErrTaskForbidden = apperr.New(apperr.CodeReviewTaskDenied)
)

// Status 是任务状态。取值与数据库 varchar 值一致。
type Status string

const (
	// StatusPending 待处理。
	StatusPending Status = "pending"
	// StatusApproved 已通过。
	StatusApproved Status = "approved"
	// StatusRejected 已拒绝。
	StatusRejected Status = "rejected"
	// StatusCanceled 已撤销（作者改回草稿或重新提交时旧任务作废）。
	StatusCanceled Status = "canceled"
)

// Stage 是审核阶段。目前只有人工审核，保留字段是为了将来接机审。
type Stage string

// StageManual 人工审核。
const StageManual Stage = "manual"

// Source 是任务的来源，决定审核列表里的文案与筛选。
type Source string

const (
	// SourceCreate 新建文章时提交。
	SourceCreate Source = "create"
	// SourceEdit 编辑已发布/审核中的文章时自动重新提交。
	SourceEdit Source = "edit"
	// SourceResubmit 作者手动重新提交。
	SourceResubmit Source = "resubmit"
)

// Task 是审核任务。
type Task struct {
	id            uint64
	articleID     domain.ArticleID
	authorID      domain.UserID
	articleTitle  string
	authorName    string
	publishStatus domain.Status
	stage         Stage
	source        Source
	status        Status
	reason        string
	reviewedAt    *time.Time
	reviewedBy    *domain.UserID
	createdAt     time.Time
}

// NewTaskParams 是创建审核任务的输入。
type NewTaskParams struct {
	ID            uint64
	ArticleID     domain.ArticleID
	AuthorID      domain.UserID
	ArticleTitle  string
	AuthorName    string
	PublishStatus domain.Status
	Source        Source
	Now           time.Time
}

// NewTask 创建待审核任务。
func NewTask(p NewTaskParams) *Task {
	return &Task{
		id:            p.ID,
		articleID:     p.ArticleID,
		authorID:      p.AuthorID,
		articleTitle:  p.ArticleTitle,
		authorName:    p.AuthorName,
		publishStatus: p.PublishStatus,
		stage:         StageManual,
		source:        p.Source,
		status:        StatusPending,
		createdAt:     p.Now,
	}
}

// RestoreTaskParams 是重建审核任务的输入。
type RestoreTaskParams struct {
	ID            uint64
	ArticleID     domain.ArticleID
	AuthorID      domain.UserID
	ArticleTitle  string
	AuthorName    string
	PublishStatus domain.Status
	Stage         Stage
	Source        Source
	Status        Status
	Reason        string
	ReviewedAt    *time.Time
	ReviewedBy    *domain.UserID
	CreatedAt     time.Time
}

// RestoreTask 从持久化数据重建。
func RestoreTask(p RestoreTaskParams) *Task {
	return &Task{
		id:            p.ID,
		articleID:     p.ArticleID,
		authorID:      p.AuthorID,
		articleTitle:  p.ArticleTitle,
		authorName:    p.AuthorName,
		publishStatus: p.PublishStatus,
		stage:         p.Stage,
		source:        p.Source,
		status:        p.Status,
		reason:        p.Reason,
		reviewedAt:    p.ReviewedAt,
		reviewedBy:    p.ReviewedBy,
		createdAt:     p.CreatedAt,
	}
}

// ID 返回任务 ID。
func (t *Task) ID() uint64 { return t.id }

// ArticleID 返回文章。
func (t *Task) ArticleID() domain.ArticleID { return t.articleID }

// AuthorID 返回作者。
func (t *Task) AuthorID() domain.UserID { return t.authorID }

// ArticleTitle 返回文章标题（快照）。
func (t *Task) ArticleTitle() string { return t.articleTitle }

// Status 返回任务状态。
func (t *Task) Status() Status { return t.status }

// Source 返回任务来源。
func (t *Task) Source() Source { return t.source }

// PublishStatus 返回提交时的文章状态。
func (t *Task) PublishStatus() domain.Status { return t.publishStatus }

// ReviewedAt 返回审核时间。
func (t *Task) ReviewedAt() *time.Time { return t.reviewedAt }

// ReviewedBy 返回审核人。
func (t *Task) ReviewedBy() *domain.UserID { return t.reviewedBy }

// IsPending 判断是否待处理。
func (t *Task) IsPending() bool { return t.status == StatusPending }

// Approve 审核通过。
func (t *Task) Approve(reviewer domain.UserID, now time.Time) error {
	if t.status != StatusPending {
		return ErrTaskHandled
	}
	t.status = StatusApproved
	return t.markReviewed(reviewer, now, "")
}

// Reject 审核拒绝，必须带原因（前端会展示给作者）。
func (t *Task) Reject(reviewer domain.UserID, reason string, now time.Time) error {
	if t.status != StatusPending {
		return ErrTaskHandled
	}
	t.status = StatusRejected
	return t.markReviewed(reviewer, now, reason)
}

// Cancel 撤销任务（作者改回草稿或重新提交时，旧任务作废）。
func (t *Task) Cancel(reason string, now time.Time) error {
	if t.status != StatusPending {
		return ErrTaskHandled
	}
	t.status = StatusCanceled
	t.reason = reason
	at := now
	t.reviewedAt = &at
	return nil
}

func (t *Task) markReviewed(reviewer domain.UserID, now time.Time, reason string) error {
	if reviewer.IsZero() {
		return ErrTaskForbidden
	}
	at := now
	by := reviewer
	t.reviewedAt = &at
	t.reviewedBy = &by
	t.reason = reason
	return nil
}

// Repository 是审核任务的持久化契约。
type Repository interface {
	// Save 新建任务。
	Save(ctx context.Context, t *Task) error
	// Update 保存任务状态变更。
	Update(ctx context.Context, t *Task) error
	// Find 读取任务。
	Find(ctx context.Context, id uint64) (*Task, error)
	// ListPendingByArticle 取某文章当前待处理的任务。
	ListPendingByArticle(ctx context.Context, articleID domain.ArticleID) ([]*Task, error)
	// List 分页列出任务（status 为空表示全部）。
	List(ctx context.Context, status *Status, offset, limit int) ([]*Task, int64, error)
}
