package mysql

import (
	"context"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/notification/domain"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// globalRow 对应 global_notif_models 表。
type globalRow struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt
	UserVisibleRule int8
	ExpireTime      time.Time
	ActionUser      uint64
	Title           string
	Icon            string
	Content         string
	Href            string
}

// TableName 固定表名。
func (globalRow) TableName() string { return "global_notif_models" }

// userStateRow 对应 user_global_notif_models 表（每个用户各自的已读/删除状态）。
type userStateRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
	MsgID     uint64
	UserID    uint64
	IsRead    bool
	ReadAt    *time.Time
}

// TableName 固定表名。
func (userStateRow) TableName() string { return "user_global_notif_models" }

// GlobalRepo 实现全局公告持久化端口。
type GlobalRepo struct {
	db *gorm.DB
}

var _ domain.GlobalRepository = (*GlobalRepo)(nil)

// NewGlobal 构造公告仓储。
func NewGlobal(db *gorm.DB) *GlobalRepo { return &GlobalRepo{db: db} }

// ListVisible 实现 domain.GlobalRepository。
func (r *GlobalRepo) ListVisible(ctx context.Context, userID uint64, userCreatedAt time.Time, page domain.Page) ([]domain.GlobalNotif, bool, error) {
	page = page.Normalize()
	var rows []globalRow
	err := r.keyword(r.baseVisible(ctx, userID, userCreatedAt), page.Key).
		Order("created_at DESC, id DESC").
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
	return toGlobals(rows), hasMore, nil
}

// ListAll 实现 domain.GlobalRepository。
func (r *GlobalRepo) ListAll(ctx context.Context, page domain.Page) ([]domain.GlobalNotif, bool, error) {
	page = page.Normalize()
	var rows []globalRow
	err := r.keyword(r.db.WithContext(ctx), page.Key).
		Order("created_at DESC, id DESC").
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
	return toGlobals(rows), hasMore, nil
}

// FindVisible 实现 domain.GlobalRepository。
func (r *GlobalRepo) FindVisible(ctx context.Context, userID uint64, ids []uint64, userCreatedAt time.Time) ([]domain.GlobalNotif, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []globalRow
	err := r.baseVisible(ctx, userID, userCreatedAt).
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toGlobals(rows), nil
}

// MarkRead 实现 domain.GlobalRepository。
//
// 只处理"可见且未删除且未读"的记录；没有个人态记录时补一条已读记录。
// 逐条处理是有意的：公告条数很少，换来的是与旧实现完全一致的计数语义。
func (r *GlobalRepo) MarkRead(ctx context.Context, userID uint64, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var states []userStateRow
	if err := r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND msg_id IN ?", userID, ids).
		Find(&states).Error; err != nil {
		return 0, err
	}
	stateMap := make(map[uint64]userStateRow, len(states))
	for _, st := range states {
		stateMap[st.MsgID] = st
	}

	success := 0
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			st, ok := stateMap[id]
			if ok && !st.DeletedAt.Valid {
				if st.IsRead {
					continue
				}
				res := tx.Model(&userStateRow{}).
					Where("msg_id = ? AND user_id = ? AND deleted_at IS NULL AND is_read = ?", id, userID, false).
					Updates(map[string]any{"is_read": true, "read_at": &now, "updated_at": now})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					success++
				}
				continue
			}
			if ok {
				// 已删除的个人态不复活，跳过。
				continue
			}
			res := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "msg_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&userStateRow{
				ID:        newStateID(),
				MsgID:     id,
				UserID:    userID,
				IsRead:    true,
				ReadAt:    &now,
				CreatedAt: now,
				UpdatedAt: now,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				success++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return success, nil
}

// DeleteForUser 实现 domain.GlobalRepository。
func (r *GlobalRepo) DeleteForUser(ctx context.Context, userID uint64, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var states []userStateRow
	if err := r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND msg_id IN ?", userID, ids).
		Find(&states).Error; err != nil {
		return 0, err
	}
	stateMap := make(map[uint64]userStateRow, len(states))
	for _, st := range states {
		stateMap[st.MsgID] = st
	}

	success := 0
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			st, ok := stateMap[id]
			if ok && !st.DeletedAt.Valid {
				res := tx.Unscoped().Model(&userStateRow{}).
					Where("msg_id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
					Updates(map[string]any{"deleted_at": now, "updated_at": now})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					success++
				}
				continue
			}
			if ok {
				continue
			}
			// 从未产生过个人态记录：直接写一条墓碑，后续列表查询据此排除。
			res := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "msg_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&userStateRow{
				ID:        newStateID(),
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
				MsgID:     id,
				UserID:    userID,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				success++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return success, nil
}

// ReadSet 实现 domain.GlobalRepository。
func (r *GlobalRepo) ReadSet(ctx context.Context, userID uint64, ids []uint64) (map[uint64]bool, error) {
	out := make(map[uint64]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var states []userStateRow
	err := r.db.WithContext(ctx).
		Select("msg_id", "is_read").
		Where("user_id = ? AND msg_id IN ?", userID, ids).
		Find(&states).Error
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if st.IsRead {
			out[st.MsgID] = true
		}
	}
	return out, nil
}

// CountUnread 实现 domain.GlobalRepository。
func (r *GlobalRepo) CountUnread(ctx context.Context, userID uint64, userCreatedAt time.Time) (int64, error) {
	var count int64
	err := r.unreadQuery(ctx, userID, userCreatedAt).Count(&count).Error
	return count, err
}

// Create 实现 domain.GlobalRepository。
func (r *GlobalRepo) Create(ctx context.Context, n *domain.GlobalNotif) error {
	if n == nil {
		return apperr.New(apperr.CodeNotifCreateFailed)
	}
	if n.ID == 0 {
		next, err := idgen.Next()
		if err != nil {
			return err
		}
		n.ID = next.Uint64()
	}
	now := time.Now()
	row := globalRow{
		ID:              n.ID,
		CreatedAt:       now,
		UpdatedAt:       now,
		UserVisibleRule: int8(n.VisibleRule.Effective()),
		ExpireTime:      n.ExpireAt,
		ActionUser:      n.ActionUser,
		Title:           n.Title,
		Icon:            n.Icon,
		Content:         n.Content,
		Href:            n.Href,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	n.CreatedAt = now
	return nil
}

// ExistsTitle 实现 domain.GlobalRepository。
func (r *GlobalRepo) ExistsTitle(ctx context.Context, title string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&globalRow{}).
		Where("title = ?", title).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteMany 实现 domain.GlobalRepository。
//
// 旧的 BeforeDelete 钩子会连带删除用户状态；适配器不能用 models，
// 所以这里在一个事务里显式做同样的事。
func (r *GlobalRepo) DeleteMany(ctx context.Context, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	deleted := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id IN ?", ids).Delete(&globalRow{})
		if res.Error != nil {
			return res.Error
		}
		deleted = int(res.RowsAffected)
		if deleted == 0 {
			return nil
		}
		return tx.Unscoped().Where("msg_id IN ?", ids).Delete(&userStateRow{}).Error
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// baseVisible 构造"该用户可见、且未被该用户删除"的公告查询。
//
// 三段 OR 与旧 SQL 逐字对应；已删除的用 NOT IN 子查询排除，
// 不再像旧实现那样把用户全部状态读进内存。
func (r *GlobalRepo) baseVisible(ctx context.Context, userID uint64, userCreatedAt time.Time) *gorm.DB {
	db := r.db.WithContext(ctx)
	now := time.Now()
	deletedSub := db.Unscoped().Model(&userStateRow{}).
		Select("msg_id").
		Where("user_id = ? AND deleted_at IS NOT NULL", userID)
	return db.Model(&globalRow{}).
		Where("expire_time > ?", now).
		Where(db.Where("user_visible_rule = ?", int8(domain.VisibleAll)).
			Or(db.Where(
				"user_visible_rule = ? AND created_at >= ? AND expire_time >= ?",
				int8(domain.VisibleRegistered), userCreatedAt, userCreatedAt,
			)).
			Or(db.Where(
				"user_visible_rule = ? AND created_at < ? AND expire_time >= ?",
				int8(domain.VisibleNew), userCreatedAt, userCreatedAt,
			))).
		Where("id NOT IN (?)", deletedSub)
}

// unreadQuery 在可见集合上再排除已读。
func (r *GlobalRepo) unreadQuery(ctx context.Context, userID uint64, userCreatedAt time.Time) *gorm.DB {
	db := r.db.WithContext(ctx)
	readSub := db.Model(&userStateRow{}).
		Select("msg_id").
		Where("user_id = ? AND is_read = ?", userID, true)
	return r.baseVisible(ctx, userID, userCreatedAt).
		Where("id NOT IN (?)", readSub)
}

// keyword 叠加标题/内容模糊匹配（与旧列表查询的 Likes 行为一致）。
func (r *GlobalRepo) keyword(db *gorm.DB, key string) *gorm.DB {
	key = strings.TrimSpace(key)
	if key == "" {
		return db
	}
	pattern := "%" + key + "%"
	return db.Where("title LIKE ? OR content LIKE ?", pattern, pattern)
}

// toGlobals 批量转换。
func toGlobals(rows []globalRow) []domain.GlobalNotif {
	out := make([]domain.GlobalNotif, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.GlobalNotif{
			ID:          row.ID,
			VisibleRule: domain.VisibleRule(row.UserVisibleRule),
			ExpireAt:    row.ExpireTime,
			ActionUser:  row.ActionUser,
			Title:       row.Title,
			Icon:        row.Icon,
			Content:     row.Content,
			Href:        row.Href,
			CreatedAt:   row.CreatedAt,
		})
	}
	return out
}

// newStateID 生成个人态记录的主键（该表主键不是自增）。
func newStateID() uint64 {
	next, err := idgen.Next()
	if err != nil {
		return 0
	}
	return next.Uint64()
}
