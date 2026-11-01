// Package mysql 是评论模块的 MySQL 适配器。
//
// 只在这一层出现表名与列名（comment_models / comment_digg_models），
// 上层拿到的都是领域实体与 apperr 错误码。
package mysql

import (
	"context"
	"myblogx/internal/kernel/ctype"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/comment/domain"
	"myblogx/internal/platform/idgen"
	platformmysql "myblogx/internal/platform/mysql"

	"gorm.io/gorm"
)

// commentRow 对应 comment_models 表。
type commentRow struct {
	ID                ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt
	Content           string
	UserID            ctype.ID
	UserNickname      string
	UserAvatar        string
	ArticleID         ctype.ID
	// ReplyID 用指针：根评论没有父评论可指向，必须落 NULL 而不是 0。
	//
	// 表里 fk_comment_models_parent_model 要求 reply_id 指向一条真实评论，
	// 而库里**没有 id=0 的行**。值类型零值会被 ctype.ID 的 Value() 写成 0，
	// 于是任何根评论都违反该外键（线上 violates foreign key constraint）。
	// NULL 不受外键约束，正是"没有父评论"的正确表达。
	ReplyID           *ctype.ID
	ReplyUserID       ctype.ID
	ReplyUserNickname string
	RootID            ctype.ID
	DiggCount         int
	ReplyCount        int
	Status            int8
}

// TableName 固定表名。
func (commentRow) TableName() string { return "comment_models" }

// diggRow 对应 comment_digg_models 表。
type diggRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	CommentID ctype.ID
	UserID    ctype.ID
}

// TableName 固定表名。
func (diggRow) TableName() string { return "comment_digg_models" }

// Repo 实现评论持久化端口。
type Repo struct {
	db *gorm.DB
}

var _ domain.Repository = (*Repo)(nil)

// New 构造评论仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// DiggRepo 实现点赞持久化端口。
//
// 拆成独立类型是因为两个端口的 Create 方法签名不同（评论 void、点赞 bool），
// Go 不允许同一个类型上出现同名方法。
type DiggRepo struct {
	db *gorm.DB
}

var _ domain.DiggRepository = (*DiggRepo)(nil)

// NewDigg 构造点赞仓储。
func NewDigg(db *gorm.DB) *DiggRepo { return &DiggRepo{db: db} }

// nullableID 把零值 ID 转成 SQL NULL，非零值原样返回。
//
// 根评论的 reply_id 必须是 NULL：写成 0 会撞 fk_comment_models_parent_model
// （库里没有 id=0 的评论），表现为 violates foreign key constraint。
func nullableID(id ctype.ID) *ctype.ID {
	if id.IsZero() {
		return nil
	}
	value := id
	return &value
}

// parentIDFromNull 把可空的父评论列读回值类型：NULL 表示"没有父评论"。
//
// 名字刻意带上 Null：读的是可能为 NULL 的列，NULL 的语义是"没有父评论"
// 而不是"父评论 ID 为 0"——后者会被外键拒绝（库里没有 id=0 的评论）。
func parentIDFromNull(id *ctype.ID) ctype.ID {
	if id == nil {
		return ctype.ID("")
	}
	return *id
}

// Create 实现 domain.Repository。
func (r *Repo) Create(ctx context.Context, c *domain.Comment) error {
	if c == nil {
		return apperr.New(apperr.CodeCommentNotFound)
	}
	if c.ID.IsZero() {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		c.ID = ctype.NewID(next.Uint64())
	}
	now := time.Now()
	row := commentRow{
		ID:                c.ID,
		CreatedAt:         now,
		Content:           c.Content,
		UserID:            c.UserID,
		UserNickname:      c.UserNickname,
		UserAvatar:        c.UserAvatar,
		ArticleID:         c.ArticleID,
		ReplyID:           nullableID(c.ReplyID),
		ReplyUserID:       c.ReplyUserID,
		ReplyUserNickname: c.ReplyUserNickname,
		RootID:            c.RootID,
		DiggCount:         c.DiggCount,
		ReplyCount:        c.ReplyCount,
		Status:            int8(c.Status),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	c.CreatedAt = now
	return nil
}

// FindByID 实现 domain.Repository。
func (r *Repo) FindByID(ctx context.Context, id domain.CommentID) (domain.Comment, error) {
	var row commentRow
	if err := r.db.WithContext(ctx).Take(&row, "id = ?", id.Uint64()).Error; err != nil {
		return domain.Comment{}, translate(err)
	}
	return toComment(row), nil
}

// FindPublished 实现 domain.Repository。
func (r *Repo) FindPublished(ctx context.Context, id domain.CommentID, articleID uint64) (domain.Comment, error) {
	var row commentRow
	if err := r.db.WithContext(ctx).
		Take(&row, "id = ? AND article_id = ? AND status = ?", id.Uint64(), articleID, int8(domain.StatusPublished)).Error; err != nil {
		return domain.Comment{}, translate(err)
	}
	return toComment(row), nil
}

// FindPublishedByID 实现 domain.Repository。
func (r *Repo) FindPublishedByID(ctx context.Context, id domain.CommentID) (domain.Comment, error) {
	var row commentRow
	if err := r.db.WithContext(ctx).
		Take(&row, "id = ? AND status = ?", id.Uint64(), int8(domain.StatusPublished)).Error; err != nil {
		return domain.Comment{}, translate(err)
	}
	return toComment(row), nil
}

// FindByIDs 实现 domain.Repository。
func (r *Repo) FindByIDs(ctx context.Context, ids []domain.CommentID) ([]domain.Comment, error) {
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !id.IsZero() {
			raw = append(raw, id.Uint64())
		}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var rows []commentRow
	if err := r.db.WithContext(ctx).Where("id IN ?", raw).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toComment(row))
	}
	return out, nil
}

// ListRoot 实现 domain.Repository。
func (r *Repo) ListRoot(ctx context.Context, articleID uint64, page domain.Page) ([]domain.Comment, bool, error) {
	page = page.Normalize()
	var rows []commentRow
	err := r.db.WithContext(ctx).
		// 父字段改成可空后，新根评论的 reply_id 是 NULL；但**存量**根评论
		// 是修复前写入的 0（零 DDL 禁止回填历史行）。因此这里必须同时接受
		// NULL 与 0，否则老根评论会一条都列不出来。
		// root_id 仍是值类型（根评论写 0），判等保持。
		Where(
			"article_id = ? AND status = ? AND (reply_id IS NULL OR reply_id = 0) AND root_id = 0",
			articleID, int8(domain.StatusPublished),
		).
		Order("created_at DESC").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return toComments(rows), hasMore, nil
}

// ListReplies 实现 domain.Repository。
func (r *Repo) ListReplies(ctx context.Context, articleID, rootID uint64, page domain.Page) ([]domain.Comment, bool, error) {
	page = page.Normalize()
	var rows []commentRow
	err := r.db.WithContext(ctx).
		Where("article_id = ? AND root_id = ? AND status = ?", articleID, rootID, int8(domain.StatusPublished)).
		Order("created_at ASC").
		Limit(page.Limit + 1).
		Offset(page.Offset()).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	return toComments(rows), hasMore, nil
}

// ListFamily 实现 domain.Repository。
func (r *Repo) ListFamily(ctx context.Context, rootID domain.CommentID) ([]domain.Comment, error) {
	var rows []commentRow
	if err := r.db.WithContext(ctx).
		Where("id = ? OR root_id = ?", rootID.Uint64(), rootID.Uint64()).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return toComments(rows), nil
}

// ListManaged 实现 domain.Repository。
func (r *Repo) ListManaged(ctx context.Context, q domain.ManagedQuery) ([]domain.Comment, int64, error) {
	page := q.Page.Normalize()

	base := func() *gorm.DB {
		tx := r.db.WithContext(ctx).Model(&commentRow{})
		switch q.Type {
		case 1:
			tx = tx.Where("article_id IN ?", q.ArticleIDs).Where("status = ?", int8(domain.StatusPublished))
		case 2:
			tx = tx.Where("user_id = ?", q.ViewerID)
		}
		if !q.ArticleID.IsZero() && q.Type != 1 {
			tx = tx.Where("article_id = ?", q.ArticleID)
		}
		if !q.UserID.IsZero() {
			tx = tx.Where("user_id = ?", q.UserID)
		}
		if q.Status != 0 && q.Type != 1 {
			tx = tx.Where("status = ?", int8(q.Status))
		}
		if page.Key != "" {
			tx = tx.Where("content LIKE ?", "%"+page.Key+"%")
		}
		return tx
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []commentRow
	if err := base().Order("created_at DESC").
		Limit(page.Limit).
		Offset((page.Page - 1) * page.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toComments(rows), total, nil
}

// UpdateStatus 实现 domain.Repository。
func (r *Repo) UpdateStatus(ctx context.Context, id domain.CommentID, status domain.Status) error {
	return r.db.WithContext(ctx).Model(&commentRow{}).
		Where("id = ?", id.Uint64()).
		Update("status", int8(status)).Error
}

// SoftDeleteMany 实现 domain.Repository。
func (r *Repo) SoftDeleteMany(ctx context.Context, ids []domain.CommentID) error {
	raw := make([]uint64, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.Uint64())
	}
	if len(raw) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", raw).Delete(&commentRow{}).Error
}

// Exists 实现 domain.DiggRepository。
func (d *DiggRepo) Exists(ctx context.Context, commentID, userID uint64) (bool, error) {
	var row diggRow
	err := d.db.WithContext(ctx).Take(&row, "comment_id = ? AND user_id = ?", commentID, userID).Error
	if err == nil {
		return true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return false, err
}

// Delete 实现 domain.DiggRepository。
func (d *DiggRepo) Delete(ctx context.Context, commentID, userID uint64) (bool, error) {
	res := d.db.WithContext(ctx).
		Where("comment_id = ? AND user_id = ?", commentID, userID).
		Delete(&diggRow{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// Create 实现 domain.DiggRepository。
func (d *DiggRepo) Create(ctx context.Context, commentID, userID uint64) (bool, error) {
	row := &diggRow{CommentID: ctype.NewID(commentID), UserID: ctype.NewID(userID)}
	if next, err := idgen.Next(); err == nil {
		row.ID = next
	}
	return platformmysql.RestoreOrCreateUnique(d.db.WithContext(ctx), row, []string{"comment_id", "user_id"})
}

// DeleteByComments 实现 domain.DiggRepository。
func (d *DiggRepo) DeleteByComments(ctx context.Context, commentIDs []uint64) error {
	if len(commentIDs) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Where("comment_id IN ?", commentIDs).Delete(&diggRow{}).Error
}

// DiggedSet 实现 domain.DiggRepository。
func (d *DiggRepo) DiggedSet(ctx context.Context, viewerID uint64, commentIDs []uint64) (map[uint64]bool, error) {
	out := make(map[uint64]bool, len(commentIDs))
	if viewerID == 0 || len(commentIDs) == 0 {
		return out, nil
	}
	var rows []diggRow
	if err := d.db.WithContext(ctx).Select("comment_id").
		Where("user_id = ? AND comment_id IN ?", viewerID, commentIDs).
		Find(&rows).Error; err != nil {
		return out, err
	}
	for _, row := range rows {
		out[row.CommentID.Uint64()] = true
	}
	return out, nil
}

// translate 把存储错误翻译成业务错误码。
func translate(err error) error {
	if err == gorm.ErrRecordNotFound {
		return apperr.New(apperr.CodeCommentNotFound)
	}
	return err
}

// toComment 把 row 映射成领域实体。
func toComment(row commentRow) domain.Comment {
	return domain.Comment{
		ID:                row.ID,
		CreatedAt:         row.CreatedAt,
		Content:           row.Content,
		UserID:            row.UserID,
		UserNickname:      row.UserNickname,
		UserAvatar:        row.UserAvatar,
		ArticleID:         row.ArticleID,
		ReplyID:           parentIDFromNull(row.ReplyID),
		ReplyUserID:       row.ReplyUserID,
		ReplyUserNickname: row.ReplyUserNickname,
		RootID:            row.RootID,
		DiggCount:         row.DiggCount,
		ReplyCount:        row.ReplyCount,
		Status:            domain.Status(row.Status),
	}
}

// toComments 批量映射。
func toComments(rows []commentRow) []domain.Comment {
	out := make([]domain.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toComment(row))
	}
	return out
}
