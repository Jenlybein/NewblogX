// Package favorite 是文章收藏子域。
//
// 两层结构：Folder（收藏夹）与 Item（收藏关系）。
// 收藏夹是用户自建的分类容器，Item 记录"谁把哪篇文章收进了哪个夹子"，
// 并冗余了文章标题/摘要/封面等快照——列表页不 join 文章表，这是既有设计，保留。
package favorite

import (
	"context"
	"myblogx/internal/kernel/ctype"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
)

// 子域错误。文案统一在 internal/apperr 的消息表里。
var (
	// ErrTitleRequired 收藏夹名称不能为空。
	ErrTitleRequired = apperr.New(apperr.CodeFavoriteTitleRequired)
	// ErrFolderNotFound 收藏夹不存在。
	ErrFolderNotFound = apperr.New(apperr.CodeFavoriteFolderMissing)
	// ErrFolderExists 收藏夹名称重复。
	ErrFolderExists = apperr.New(apperr.CodeFavoriteFolderExists)
	// ErrFolderForbidden 无权操作该收藏夹。
	ErrFolderForbidden = apperr.New(apperr.CodeFavoriteFolderDenied)
	// ErrItemExists 已收藏该文章。
	ErrItemExists = apperr.New(apperr.CodeFavoriteExists)
	// ErrItemNotFound 尚未收藏该文章。
	ErrItemNotFound = apperr.New(apperr.CodeFavoriteNotFound)
	// ErrSelfFavorite 不能收藏自己的文章。
	ErrSelfFavorite = apperr.New(apperr.CodeFavoriteSelf)
)

// ID 是收藏夹标识。
type ID = ctype.ID

// Folder 是收藏夹。
//
// 唯一约束 (userID, title)，与 uk_favorite_user_title 一致。
// IsDefault 标记默认收藏夹：用户首次收藏时自动落入，不允许删除。
type Folder struct {
	id           ID
	userID       domain.UserID
	title        string
	cover        string
	abstract     string
	isDefault    bool
	articleCount int

	// 读模型字段：列表页展示时间、所属用户与"是否已收藏当前文章"。
	createdAt     time.Time
	updatedAt     time.Time
	ownerNickname string
	ownerAvatar   string
	hasArticle    bool
}

// NewFolder 创建收藏夹。
func NewFolder(id ID, userID domain.UserID, title string) (*Folder, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return nil, ErrTitleRequired
	}
	if userID.IsZero() {
		return nil, ErrFolderForbidden
	}
	return &Folder{id: id, userID: userID, title: trimmed}, nil
}

// RestoreFolderParams 是重建收藏夹的输入。
type RestoreFolderParams struct {
	ID            ID
	UserID        domain.UserID
	Title         string
	Cover         string
	Abstract      string
	IsDefault     bool
	ArticleCount  int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	OwnerNickname string
	OwnerAvatar   string
	HasArticle    bool
}

// RestoreFolder 从持久化数据重建。
func RestoreFolder(p RestoreFolderParams) *Folder {
	return &Folder{
		id:            p.ID,
		userID:        p.UserID,
		title:         p.Title,
		cover:         p.Cover,
		abstract:      p.Abstract,
		isDefault:     p.IsDefault,
		articleCount:  p.ArticleCount,
		createdAt:     p.CreatedAt,
		updatedAt:     p.UpdatedAt,
		ownerNickname: p.OwnerNickname,
		ownerAvatar:   p.OwnerAvatar,
		hasArticle:    p.HasArticle,
	}
}

// ID 返回收藏夹 ID。
func (f *Folder) ID() ID { return f.id }

// UserID 返回所属用户。
func (f *Folder) UserID() domain.UserID { return f.userID }

// Title 返回名称。
func (f *Folder) Title() string { return f.title }

// Cover 返回封面。
func (f *Folder) Cover() string { return f.cover }

// Abstract 返回简介。
func (f *Folder) Abstract() string { return f.abstract }

// IsDefault 返回是否默认收藏夹。
func (f *Folder) IsDefault() bool { return f.isDefault }

// ArticleCount 返回文章数。
func (f *Folder) ArticleCount() int { return f.articleCount }

// CreatedAt 返回创建时间（读模型）。
func (f *Folder) CreatedAt() time.Time { return f.createdAt }

// UpdatedAt 返回更新时间（读模型）。
func (f *Folder) UpdatedAt() time.Time { return f.updatedAt }

// OwnerNickname 返回所属用户昵称（读模型）。
func (f *Folder) OwnerNickname() string { return f.ownerNickname }

// OwnerAvatar 返回所属用户头像（读模型）。
func (f *Folder) OwnerAvatar() string { return f.ownerAvatar }

// HasArticle 返回当前查询的那篇文章是否已在该夹内（读模型）。
func (f *Folder) HasArticle() bool { return f.hasArticle }

// Rename 改名并更新封面/简介。
func (f *Folder) Rename(title, cover, abstract string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrTitleRequired
	}
	f.title = trimmed
	f.cover = cover
	f.abstract = abstract
	return nil
}

// BelongsTo 判断收藏夹是否属于某用户。
func (f *Folder) BelongsTo(userID domain.UserID) bool { return f.userID == userID }

// CanDelete 判断收藏夹能否删除：默认收藏夹不可删。
func (f *Folder) CanDelete() bool { return !f.isDefault }

// Item 是一条收藏关系（含文章快照）。
type Item struct {
	articleID domain.ArticleID
	userID    domain.UserID
	folderID  ID

	articleTitle    string
	articleAbstract string
	articleCover    string
	articleStatus   domain.Status
	authorID        domain.UserID
	authorNickname  string
	authorAvatar    string

	// 读模型字段：收藏时间与文章实时计数（列表页直接展示）。
	favoritedAt  time.Time
	viewCount    int
	diggCount    int
	commentCount int
	favorCount   int
}

// RestoreItemParams 是重建收藏关系的输入。
type RestoreItemParams struct {
	ArticleID       domain.ArticleID
	UserID          domain.UserID
	FolderID        ID
	ArticleTitle    string
	ArticleAbstract string
	ArticleCover    string
	ArticleStatus   domain.Status
	AuthorID        domain.UserID
	AuthorNickname  string
	AuthorAvatar    string
	FavoritedAt     time.Time
	ViewCount       int
	DiggCount       int
	CommentCount    int
	FavorCount      int
}

// RestoreItem 从持久化数据重建。
func RestoreItem(p RestoreItemParams) *Item {
	return &Item{
		articleID:       p.ArticleID,
		userID:          p.UserID,
		folderID:        p.FolderID,
		articleTitle:    p.ArticleTitle,
		articleAbstract: p.ArticleAbstract,
		articleCover:    p.ArticleCover,
		articleStatus:   p.ArticleStatus,
		authorID:        p.AuthorID,
		authorNickname:  p.AuthorNickname,
		authorAvatar:    p.AuthorAvatar,
		favoritedAt:     p.FavoritedAt,
		viewCount:       p.ViewCount,
		diggCount:       p.DiggCount,
		commentCount:    p.CommentCount,
		favorCount:      p.FavorCount,
	}
}

// NewItemFromArticle 用文章当前状态生成一条收藏快照。
//
// 快照字段全部从聚合根拷过来，收藏列表因此不需要 join 文章表。
func NewItemFromArticle(folderID ID, userID domain.UserID, a *domain.Article) *Item {
	return &Item{
		articleID:       a.ID(),
		userID:          userID,
		folderID:        folderID,
		articleTitle:    a.Title(),
		articleAbstract: a.Abstract(),
		articleCover:    a.Cover(),
		articleStatus:   a.Status(),
		authorID:        a.AuthorID(),
	}
}

// ArticleID 返回被收藏的文章。
func (i *Item) ArticleID() domain.ArticleID { return i.articleID }

// UserID 返回收藏者。
func (i *Item) UserID() domain.UserID { return i.userID }

// FolderID 返回所属收藏夹。
func (i *Item) FolderID() ID { return i.folderID }

// ArticleTitle 返回文章标题快照。
func (i *Item) ArticleTitle() string { return i.articleTitle }

// ArticleAbstract 返回文章摘要快照。
func (i *Item) ArticleAbstract() string { return i.articleAbstract }

// ArticleCover 返回文章封面快照。
func (i *Item) ArticleCover() string { return i.articleCover }

// ArticleStatus 返回收藏时的文章状态。
func (i *Item) ArticleStatus() domain.Status { return i.articleStatus }

// AuthorID 返回作者。
func (i *Item) AuthorID() domain.UserID { return i.authorID }

// AuthorNickname 返回作者昵称快照。
func (i *Item) AuthorNickname() string { return i.authorNickname }

// AuthorAvatar 返回作者头像快照。
func (i *Item) AuthorAvatar() string { return i.authorAvatar }

// FavoritedAt 返回收藏时间（读模型）。
func (i *Item) FavoritedAt() time.Time { return i.favoritedAt }

// ViewCount 返回文章浏览量（读模型）。
func (i *Item) ViewCount() int { return i.viewCount }

// DiggCount 返回文章点赞数（读模型）。
func (i *Item) DiggCount() int { return i.diggCount }

// CommentCount 返回文章评论数（读模型）。
func (i *Item) CommentCount() int { return i.commentCount }

// FavorCount 返回文章收藏数（读模型）。
func (i *Item) FavorCount() int { return i.favorCount }

// WithAuthorProfile 补全作者昵称与头像（调用方从用户模块取）。
func (i *Item) WithAuthorProfile(nickname, avatar string) *Item {
	i.authorNickname = nickname
	i.authorAvatar = avatar
	return i
}

// Repository 是收藏子域的持久化契约。
type Repository interface {
	// FindFolder 读取收藏夹。
	FindFolder(ctx context.Context, id ID) (*Folder, error)
	// DefaultFolder 取某用户的默认收藏夹；没有则创建。
	DefaultFolder(ctx context.Context, userID domain.UserID) (*Folder, error)
	// ListFolders 列出某用户的收藏夹。
	ListFolders(ctx context.Context, userID domain.UserID) ([]*Folder, error)
	// ExistsFolderTitle 判断名称是否重复。
	ExistsFolderTitle(ctx context.Context, userID domain.UserID, title string, excludeID ID) (bool, error)
	// SaveFolder 新建或更新收藏夹。
	SaveFolder(ctx context.Context, f *Folder) error
	// DeleteFolder 删除收藏夹与其下所有收藏关系。
	DeleteFolder(ctx context.Context, id ID) error

	// FindItem 取某用户对某文章在某夹子里的收藏；不存在返回 ErrFavoriteNotFound。
	FindItem(ctx context.Context, articleID domain.ArticleID, userID domain.UserID, folderID ID) (*Item, error)
	// FindItemAnyFolder 取某用户对某文章的任意收藏（用于判断"是否已收藏"）。
	FindItemAnyFolder(ctx context.Context, articleID domain.ArticleID, userID domain.UserID) (*Item, error)
	// SaveItem 新建收藏关系。
	SaveItem(ctx context.Context, i *Item) error
	// DeleteItem 删除收藏关系。
	DeleteItem(ctx context.Context, articleID domain.ArticleID, userID domain.UserID, folderID ID) error
	// DeleteItemsByArticle 删除某文章的全部收藏（删文章级联用）。
	DeleteItemsByArticle(ctx context.Context, articleID domain.ArticleID) error
	// ListItems 分页列出某夹子里的收藏。
	//
	// onlyAdminAuthors 为真时只返回"作者是管理员"的条目（ADR-0017 决策 4：
	// 「显示非管理员发布文章」开关关闭时收藏内容同步过滤）。做成查询层条件
	// 而不是调用方查完再筛：total 必须与过滤后的可见条目一致，否则前端分页错乱。
	ListItems(ctx context.Context, folderID ID, offset, limit int, onlyAdminAuthors bool) ([]*Item, int64, error)
	// CountByArticle 统计某文章被收藏的次数（删文章时用于审计与统计扣减）。
	CountByArticle(ctx context.Context, articleID domain.ArticleID) (int, error)
}
