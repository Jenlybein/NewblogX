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
//
// 只有两种：已发布 / 已删除（票 400）。评论不再有自己的审核系统 —— 命中词在
// 落库前就被替换成 `*` 并进关键词风险页（ADR-0032 决策 5），不再有「审核中」
// 这个中间态，因此也没有"等一个不会来的审核"的评论。
//
// **1 是一个保留值**：它是已退场的「审核中」。存量行由启动时的一次性迁移转成
// 已删除（见 comment/adapter/mysql 的 RetireExamining），代码里不再写入它。
type Status int8

const (
	// StatusPublished 已发布。
	StatusPublished Status = 2
	// StatusDeleted 已删除。
	StatusDeleted Status = 3
)

// IsPublished 判断是否已发布（只有已发布才计入前台计数）。
func (s Status) IsPublished() bool { return s == StatusPublished }

// IsValid 判断状态是否在枚举范围内。
func (s Status) IsValid() bool { return s == StatusPublished || s == StatusDeleted }

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
	// DeletedAt 是软删时间；nil 表示这条评论还活着。
	//
	// 必须单独有这一格：删除动作（Repository.SoftDeleteMany）写的是 deleted_at，
	// **不改 status**，所以只看 Status 会把"已经删掉的评论"当成已发布。读者侧
	// 列表靠查询作用域天然排除了已删行，只有管理端（要翻已删除评论）才依赖它。
	DeletedAt *time.Time
}

// IsRoot 判断是否一级评论。
func (c Comment) IsRoot() bool { return c.ReplyID.IsZero() && c.RootID.IsZero() }

// IsDeleted 判断这条评论是否已被删除。
//
// 两个判据缺一不可：
//   - deleted_at 非空：正常删除路径（SoftDeleteMany）写的就是它；
//   - status = 已删除：存量「审核中」评论在启动迁移里只改了状态、没有写
//     deleted_at（见 RetireExamining），它们同样不该出现在"已发布"视图里。
//
// 管理端列表用 EffectiveStatus 出对外状态，就是为了让这两个来源在 API 上
// 收敛成同一个"已删除"。
func (c Comment) IsDeleted() bool { return c.Status == StatusDeleted || c.DeletedAt != nil }

// EffectiveStatus 返回对外展示的状态：被软删过的评论一律报告"已删除"。
//
// 直接回 Status 会把"deleted_at 非空、status=2"的行显示成已发布 —— 那正是
// 删除路径只写软删标记留下的形状。状态是给界面读的，界面不该看到这种错位。
func (c Comment) EffectiveStatus() Status {
	if c.IsDeleted() {
		return StatusDeleted
	}
	return c.Status
}

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
