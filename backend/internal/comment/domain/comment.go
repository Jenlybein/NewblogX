// Package domain 是评论模块的领域层。
//
// 只管"评论是什么、谁能删、什么时候计数"，不碰数据库、不碰 HTTP。
package domain

import (
	"myblogx/internal/kernel/ctype"
	"time"
)

// CommentID 是评论标识。
type CommentID = ctype.ID

// Status 是评论状态。
type Status int8

const (
	// StatusExamining 审核中。
	StatusExamining Status = 1
	// StatusPublished 已发布。
	StatusPublished Status = 2
	// StatusDeleted 已删除。
	StatusDeleted Status = 3
)

// IsPublished 判断是否已发布（只有已发布才计入前台计数）。
func (s Status) IsPublished() bool { return s == StatusPublished }

// IsValid 判断状态是否在枚举范围内。
func (s Status) IsValid() bool { return s >= StatusExamining && s <= StatusDeleted }

// Comment 是一条评论。
//
// 只做两级：一级评论 ReplyID/RootID 都为 0；二级评论 RootID 指向所属一级评论，
// 回复二级评论时也只把 RootID 指到同一根，不再加深层级。
type Comment struct {
	ID                CommentID
	CreatedAt         time.Time
	Content           string
	UserID            ctype.ID
	UserNickname      string
	UserAvatar        string
	ArticleID         ctype.ID
	ReplyID           ctype.ID
	ReplyUserID       ctype.ID
	ReplyUserNickname string
	RootID            ctype.ID
	DiggCount         int
	ReplyCount        int
	Status            Status
}

// IsRoot 判断是否一级评论。
func (c Comment) IsRoot() bool { return c.ReplyID.IsZero() && c.RootID.IsZero() }

// CanDelete 判断操作者能否删除这条评论。
//
// 三种人可删：管理员、评论作者、文章作者。规则放在领域层，
// 这样"谁能删"不依赖 handler 有没有记得判断。
func (c Comment) CanDelete(viewerID uint64, admin bool, articleAuthorID uint64) bool {
	if admin {
		return true
	}
	if viewerID == 0 {
		return false
	}
	return c.UserID.Uint64() == viewerID || articleAuthorID == viewerID
}

// Viewer 是评论操作者。
type Viewer struct {
	ID    ctype.ID
	Admin bool
}

// IsZero 判断是否匿名。
func (v Viewer) IsZero() bool { return v.ID.IsZero() }

// Page 是评论列表分页参数。
type Page struct {
	Page  int
	Limit int
	Key   string
}

// Normalize 收敛分页参数（与旧接口口径一致：默认 10、上限 100）。
func (p Page) Normalize() Page {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 10
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	return p
}

// Offset 返回 SQL 偏移量。
func (p Page) Offset() int { return (p.Normalize().Page - 1) * p.Normalize().Limit }
