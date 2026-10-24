// Package favorite 是文章收藏子域。
//
// 两层结构：Folder（收藏夹）与 Item（收藏关系）。
// 收藏夹是用户自建的分类容器，Item 记录"谁把哪篇文章收进了哪个夹子"，
// 并冗余了文章标题/摘要/封面等快照——列表页不 join 文章表，这是既有设计，保留。
package favorite

import (
	"context"
	"errors"
	"strings"

	"myblogx/internal/article/domain"
)

// 子域错误。
var (
	// ErrTitleRequired 收藏夹名称不能为空。
	ErrTitleRequired = errors.New("收藏夹名称不能为空")
	// ErrFolderNotFound 收藏夹不存在。
	ErrFolderNotFound = errors.New("收藏夹不存在")
	// ErrFolderExists 收藏夹名称重复。
	ErrFolderExists = errors.New("收藏夹名称重复")
	// ErrFolderForbidden 无权操作该收藏夹。
	ErrFolderForbidden = errors.New("无权操作该收藏夹")
)

// ID 是收藏夹标识。
type ID uint64

// Uint64 返回底层值。
func (id ID) Uint64() uint64 { return uint64(id) }

// IsZero 判断是否为空。
func (id ID) IsZero() bool { return id == 0 }

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
	ID           ID
	UserID       domain.UserID
	Title        string
	Cover        string
	Abstract     string
	IsDefault    bool
	ArticleCount int
}

// RestoreFolder 从持久化数据重建。
func RestoreFolder(p RestoreFolderParams) *Folder {
	return &Folder{
		id:           p.ID,
		userID:       p.UserID,
		title:        p.Title,
		cover:        p.Cover,
		abstract:     p.Abstract,
		isDefault:    p.IsDefault,
		articleCount: p.ArticleCount,
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
	ListItems(ctx context.Context, folderID ID, offset, limit int) ([]*Item, int64, error)
}
