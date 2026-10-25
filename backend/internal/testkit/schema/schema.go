// Package schema 是集成测试的**建表与造数据**集中来源。
//
// # 为什么需要它
//
// 业务模块被 R10 禁止 import myblogx/models（row 结构应放在本模块 adapter）。
// 但集成测试要建表、要塞种子数据，天然会碰到这些 row 结构。
// 过去 7 个测试文件各自 import models 自己 AutoMigrate，造成两个后果：
//
//   - R10 在测试文件上是 10 处违规，规则红着，只能靠"默认跳过测试"绕过去；
//   - M6 要删 models 时，这 10 处分散在 7 个文件里，改造点不可枚举。
//
// testkit 不是业务模块（archcheck 的 moduleOf 对它返回空），
// 所以 models 的依赖收在这里**不违规**，而且让"删 models"变成单点改造：
// M6 只需重写本包，各测试文件一行不用动。
//
// # 不复制结构体，用类型别名
//
// 曾经考虑过"把 row 结构复制一份进 testkit"，这是错的方向：复制会漂移，
// 而漂移的表现是"测试通过但线上列名错了"。这里用 type alias 直接复用，
// 零复制、零漂移；代价是 M6 删 models 时本包必须同步改 —— 那正是我们想要的
// 提醒，而不是把问题藏起来。
package schema

import (
	"testing"
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"

	"gorm.io/gorm"
)

// ── 类型别名：让调用方不必 import models ──────────────────────────

// 角色。
type RoleType = enum.RoleType

// 角色取值。
const (
	RoleAdmin = enum.RoleAdmin
	RoleUser  = enum.RoleUser
)

// 文章发布状态。
type ArticleStatus = enum.ArticleStatus

// 文章发布状态取值。
const (
	ArticleStatusDraft     = enum.ArticleStatusDraft
	ArticleStatusExamining = enum.ArticleStatusExamining
	ArticleStatusPublished = enum.ArticleStatusPublished
	ArticleStatusRejected  = enum.ArticleStatusRejected
)

// 文章可见性。
type ArticleVisibility = enum.ArticleVisibilityStatus

// 文章可见性取值。
const (
	VisibilityVisible     = enum.ArticleVisibilityVisible
	VisibilityUserHidden  = enum.ArticleVisibilityUserHidden
	VisibilityAdminHidden = enum.ArticleVisibilityAdminHidden
)

// 审核任务的三组枚举。
type (
	ReviewTaskStage  = models.ArticleReviewTaskStage
	ReviewTaskSource = models.ArticleReviewTaskSource
	ReviewTaskStatus = models.ArticleReviewTaskStatus
)

// 审核任务取值。
const (
	ReviewStageManual    = models.ArticleReviewTaskStageManual
	ReviewSourceCreate   = models.ArticleReviewTaskSourceCreate
	ReviewSourceEdit     = models.ArticleReviewTaskSourceEdit
	ReviewSourceResubmit = models.ArticleReviewTaskSourceResubmit
	ReviewStatusPending  = models.ArticleReviewTaskPending
	ReviewStatusApproved = models.ArticleReviewTaskApproved
	ReviewStatusRejected = models.ArticleReviewTaskRejected
	ReviewStatusCanceled = models.ArticleReviewTaskCanceled
)

// OutboxRow 是发件箱表的行类型（测试要断言中继改过哪些列）。
type OutboxRow = models.OutboxEventModel

// outbox 状态取值。
const (
	OutboxStatusPending    = models.OutboxStatusPending
	OutboxStatusProcessing = models.OutboxStatusProcessing
	OutboxStatusDone       = models.OutboxStatusDone
	OutboxStatusFailed     = models.OutboxStatusFailed
	OutboxStatusDead       = models.OutboxStatusDead
)

// ArticleRow 是文章表的行类型（少数测试要直接按列读数，验证投影）。
type ArticleRow = models.ArticleModel

// ── 建表 ────────────────────────────────────────────────────────

// Group 是一组建表单元。分组而不是"一把全建"，因为：
//   - MySQL 下 AutoMigrate 的顺序影响外键创建，乱序会报错；
//   - 每个测试只该建自己用得到的表，多建会掩盖"漏依赖"的问题。
type Group string

// 可用的表组。
const (
	// UserCore 是 user / user_conf / user_stat 三张表。
	// user_models 被大量表用外键引用，所以几乎每个测试都要它。
	UserCore Group = "user_core"
	// UserSession 是登录会话表。
	UserSession Group = "user_session"
	// ArticleCore 是分类 / 标签 / 文章 / 文章标签 / 文章点赞。
	ArticleCore Group = "article_core"
	// ArticleReview 是审核任务与审核日志。
	ArticleReview Group = "article_review"
	// Favorite 是收藏夹与收藏关系。
	Favorite Group = "favorite"
	// UserReading 是置顶与浏览历史。
	UserReading Group = "user_reading"
	// Comment 是评论表。
	Comment Group = "comment"
	// Chat 是会话与消息表。
	Chat Group = "chat"
	// Outbox 是事务型发件箱。
	Outbox Group = "outbox"
)

// groupModels 是每个表组对应的行结构，**组内顺序即建表顺序**。
var groupModels = map[Group][]any{
	UserCore: {
		&models.UserModel{},
		&models.UserConfModel{},
		&models.UserStatModel{},
	},
	UserSession: {
		&models.UserSessionModel{},
	},
	// ArticleCore 含 models.ArticleTagModel，因此 article_tag_models 会带上
	// id / created_at / updated_at / deleted_at 四列（不只是 (article_id, tag_id)）。
	//
	// 这是刻意的：标签仓储（article/adapter/mysql 的 ReplaceTags）用
	// ArticleTagModel 结构体读写这张表，表里没有 id 列会报 Unknown column 'id'。
	// 代价是插入时必须显式给 id —— 该列 NOT NULL 且无默认值，
	// 只写两列会报 Error 1364。见 InsertArticleTag。
	ArticleCore: {
		&models.CategoryModel{},
		&models.TagModel{},
		&models.ArticleModel{},
		&models.ArticleTagModel{},
		&models.ArticleDiggModel{},
	},
	ArticleReview: {
		&models.ArticleReviewTaskModel{},
		&models.ArticleReviewLogModel{},
	},
	Favorite: {
		&models.FavoriteModel{},
		&models.UserArticleFavorModel{},
	},
	UserReading: {
		&models.UserTopArticleModel{},
		&models.UserArticleViewHistoryModel{},
	},
	Comment: {
		&models.CommentModel{},
	},
	// Chat 里消息表必须排在会话表**之前**。
	//
	// ChatMsgModel.SessionList 是一条 hasMany，它会在 chat_session_models 上
	// 建一条指向 chat_msg_models.session_id 的外键。先建会话表的话，
	// 被引用的表还不存在，MySQL 直接报 Error 1824。
	Chat: {
		&models.ChatMsgModel{},
		&models.ChatSessionModel{},
		&models.ChatMsgUserStateModel{},
	},
	Outbox: {
		&models.OutboxEventModel{},
	},
}

// Models 返回表组对应的行结构，供 testutil.SetupSQLite 之类"接受模型列表"的入口使用。
//
// 参数顺序就是建表顺序；重复的组会被忽略（调用方不必去重）。
func Models(groups ...Group) []any {
	seen := make(map[Group]bool, len(groups))
	var out []any
	for _, g := range groups {
		if seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, groupModels[g]...)
	}
	return out
}

// Migrate 按给定顺序建表，失败即终止测试。
func Migrate(t *testing.T, db *gorm.DB, groups ...Group) {
	t.Helper()
	if err := db.AutoMigrate(Models(groups...)...); err != nil {
		t.Fatalf("建表失败（组=%v）: %v", groups, err)
	}
}

// MigrateNoFK 同 Migrate，但不创建外键约束。
//
// 只有确实被外键坑到才用它，用例见私信：
//
//	ChatMsgModel.SessionList 声明成 hasMany（foreignKey:SessionID），
//	GORM 因此在 chat_session_models.session_id 上建了一条指向
//	chat_msg_models.session_id 的外键。可是发第一条消息时**必须先有会话行、
//	再有消息行**（SendMessage 就是这么写的），这条外键让第一步必然失败：
//	Error 1452 fk_chat_msg_models_session_list。
//
// 也就是说这条约束在物理上不可能被满足，是建模的产物而不是业务规则
// （chat_msg_models.session_id 也不是唯一键，本身就不该被引用）。
// 真库若真带着它，私信从第一条消息起就是坏的。
func MigrateNoFK(t *testing.T, db *gorm.DB, groups ...Group) {
	t.Helper()
	// 复制一份 Config 再改：直接改 db.Config 会污染并发测试共享的配置。
	cfg := *db.Config
	cfg.DisableForeignKeyConstraintWhenMigrating = true
	noFK := db.Session(&gorm.Session{})
	noFK.Config = &cfg

	if err := noFK.AutoMigrate(Models(groups...)...); err != nil {
		t.Fatalf("建表失败（组=%v，无外键）: %v", groups, err)
	}
}

// ── 造数据 ──────────────────────────────────────────────────────

// UserSpec 描述要插入的用户行。
//
// 只列测试真正会用到的字段：不用的字段留在零值，比"什么都可配"更难用错。
type UserSpec struct {
	ID       uint64
	Username string
	Nickname string
	Avatar   string
	Abstract string
	// Password 为空时填 "x"（测试不校验密码）。
	Password string
	Role     RoleType
	// CreatedAt 为零值时交给 GORM（time.Now）。
	// golden 类测试必须写死，否则响应里的时间每次都变。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// InsertUser 插入一个用户行。
//
// 注意 UserModel.AfterCreate 会自动补建 user_conf / user_stat，
// 所以调用前必须已经建好这两张表（schema.UserCore 组包含它们）。
func InsertUser(t *testing.T, db *gorm.DB, spec UserSpec) {
	t.Helper()
	u := models.UserModel{}
	u.ID = ctype.ID(spec.ID)
	u.Username = spec.Username
	u.Nickname = spec.Nickname
	u.Avatar = spec.Avatar
	u.Abstract = spec.Abstract
	u.Password = spec.Password
	if u.Password == "" {
		u.Password = "x"
	}
	u.Role = spec.Role
	u.CreatedAt = spec.CreatedAt
	u.UpdatedAt = spec.UpdatedAt
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("插入用户 %d 失败: %v", spec.ID, err)
	}
}

// ArticleSpec 描述要插入的文章行。
type ArticleSpec struct {
	ID         uint64
	Title      string
	Abstract   string
	Content    string
	CategoryID *ctype.ID
	AuthorID   uint64
	ViewCount  int
	DiggCount  int
	// CommentCount / FavorCount 同理，都是冗余计数列。
	CommentCount int
	FavorCount   int
	// CommentsToggleZero 为 false 时保持字段默认值（true）。
	// 需要显式 false 时把它设为 true 并在插入后由调用方自行 UPDATE ——
	// 与 Tag 不同，文章的 comments_toggle 在测试里从没被关过，不做特殊处理。
	CommentsToggle bool
	PublishStatus  ArticleStatus
	Visibility     ArticleVisibility
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// InsertArticle 插入一篇文章行。
//
// Omit 掉关联字段：它们会让 GORM 顺带 upsert 用户/分类，把种子数据搅乱。
func InsertArticle(t *testing.T, db *gorm.DB, spec ArticleSpec) {
	t.Helper()
	m := models.ArticleModel{
		Model: models.Model{
			ID:        ctype.ID(spec.ID),
			CreatedAt: spec.CreatedAt,
			UpdatedAt: spec.UpdatedAt,
		},
		Title:            spec.Title,
		Abstract:         spec.Abstract,
		Content:          spec.Content,
		CategoryID:       spec.CategoryID,
		AuthorID:         ctype.ID(spec.AuthorID),
		ViewCount:        spec.ViewCount,
		DiggCount:        spec.DiggCount,
		CommentCount:     spec.CommentCount,
		FavorCount:       spec.FavorCount,
		CommentsToggle:   spec.CommentsToggle,
		PublishStatus:    spec.PublishStatus,
		VisibilityStatus: spec.Visibility,
	}
	if err := db.Omit("Tags", "UserModel", "CategoryModel").Create(&m).Error; err != nil {
		t.Fatalf("插入文章 %d 失败: %v", spec.ID, err)
	}
}

// InsertCategory 插入一个分类行。
func InsertCategory(t *testing.T, db *gorm.DB, id, userID uint64, title string, at time.Time) {
	t.Helper()
	m := models.CategoryModel{
		Model:  models.Model{ID: ctype.ID(id), CreatedAt: at, UpdatedAt: at},
		Title:  title,
		UserID: ctype.ID(userID),
	}
	if err := db.Omit("UserModel", "ArticleList").Create(&m).Error; err != nil {
		t.Fatalf("插入分类 %d 失败: %v", id, err)
	}
}

// TagSpec 描述要插入的标签行。
type TagSpec struct {
	ID           uint64
	Title        string
	Sort         int
	ArticleCount int
	// Enabled 为 false 时要显式 UPDATE 一次：is_enabled 带 gorm default:true，
	// 零值 false 会被数据库默认值顶回来（GORM 不写零值）。
	Enabled   bool
	CreatedBy uint64
	At        time.Time
}

// InsertTag 插入一个标签行。
func InsertTag(t *testing.T, db *gorm.DB, spec TagSpec) {
	t.Helper()
	m := models.TagModel{
		Model:        models.Model{ID: ctype.ID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		Title:        spec.Title,
		Sort:         spec.Sort,
		ArticleCount: spec.ArticleCount,
		IsEnabled:    spec.Enabled,
		CreatedBy:    ctype.ID(spec.CreatedBy),
	}
	if err := db.Omit("ArticleList").Create(&m).Error; err != nil {
		t.Fatalf("插入标签 %d 失败: %v", spec.ID, err)
	}
	if !spec.Enabled {
		// 见 TagSpec.Enabled 的说明：默认值会把 false 顶回来。
		if err := db.Model(&models.TagModel{}).Where("id = ?", spec.ID).
			Update("is_enabled", false).Error; err != nil {
			t.Fatalf("停用标签 %d 失败: %v", spec.ID, err)
		}
	}
}

// InsertArticleTag 建立文章与标签的关系。
//
// 用原生 SQL 而不是 ArticleTagModel 结构体：结构体带关联字段，
// GORM 会尝试 upsert 关联行，把种子数据搅乱。
// 显式写 id：该列 NOT NULL 且无默认值，省略会报 Error 1364。
//
// id 用 articleID*1000+tagID 而不取雪花值，是为了让种子完全可复现 ——
// golden 的种子表里不该出现随机数，哪怕这个 id 目前不进响应。
func InsertArticleTag(t *testing.T, db *gorm.DB, articleID, tagID uint64) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO article_tag_models (id, article_id, tag_id) VALUES (?, ?, ?)",
		articleID*1000+tagID, articleID, tagID,
	).Error; err != nil {
		t.Fatalf("建立文章标签关系 %d/%d 失败: %v", articleID, tagID, err)
	}
}

// FavoriteSpec 描述要插入的收藏夹行。
type FavoriteSpec struct {
	ID           uint64
	UserID       uint64
	Title        string
	IsDefault    bool
	ArticleCount int
	At           time.Time
}

// InsertFavorite 插入一个收藏夹行。
func InsertFavorite(t *testing.T, db *gorm.DB, spec FavoriteSpec) {
	t.Helper()
	m := models.FavoriteModel{
		Model:        models.Model{ID: ctype.ID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		UserID:       ctype.ID(spec.UserID),
		Title:        spec.Title,
		IsDefault:    spec.IsDefault,
		ArticleCount: spec.ArticleCount,
	}
	if err := db.Omit("UserModel", "ArticleList").Create(&m).Error; err != nil {
		t.Fatalf("插入收藏夹 %d 失败: %v", spec.ID, err)
	}
}

// UserArticleFavorSpec 描述要插入的收藏关系行（带文章快照列）。
type UserArticleFavorSpec struct {
	ID                    uint64
	ArticleID             uint64
	UserID                uint64
	FavorID               uint64
	ArticleTitle          string
	ArticleAbstract       string
	ArticleStatus         ArticleStatus
	ArticleAuthorID       uint64
	ArticleAuthorNickname string
	At                    time.Time
}

// InsertUserArticleFavor 插入一条收藏关系。
func InsertUserArticleFavor(t *testing.T, db *gorm.DB, spec UserArticleFavorSpec) {
	t.Helper()
	m := models.UserArticleFavorModel{
		Model:                 models.Model{ID: ctype.ID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		ArticleID:             ctype.ID(spec.ArticleID),
		UserID:                ctype.ID(spec.UserID),
		FavorID:               ctype.ID(spec.FavorID),
		ArticleTitle:          spec.ArticleTitle,
		ArticleAbstract:       spec.ArticleAbstract,
		ArticleStatus:         spec.ArticleStatus,
		ArticleAuthorID:       ctype.ID(spec.ArticleAuthorID),
		ArticleAuthorNickname: spec.ArticleAuthorNickname,
	}
	if err := db.Omit("UserModel", "ArticleModel", "FavoriteModel").Create(&m).Error; err != nil {
		t.Fatalf("插入收藏关系 %d 失败: %v", spec.ArticleID, err)
	}
}

// InsertUserTopArticle 插入一条置顶记录。
func InsertUserTopArticle(t *testing.T, db *gorm.DB, id, userID, articleID uint64, at time.Time) {
	t.Helper()
	m := models.UserTopArticleModel{
		Model:     models.Model{ID: ctype.ID(id), CreatedAt: at, UpdatedAt: at},
		UserID:    ctype.ID(userID),
		ArticleID: ctype.ID(articleID),
	}
	if err := db.Omit("UserModel", "ArticleModel").Create(&m).Error; err != nil {
		t.Fatalf("插入置顶 %d 失败: %v", id, err)
	}
}

// InsertViewHistory 插入一条浏览历史。
func InsertViewHistory(t *testing.T, db *gorm.DB, id, userID, articleID uint64, at time.Time) {
	t.Helper()
	m := models.UserArticleViewHistoryModel{
		Model:     models.Model{ID: ctype.ID(id), CreatedAt: at, UpdatedAt: at},
		UserID:    ctype.ID(userID),
		ArticleID: ctype.ID(articleID),
	}
	if err := db.Omit("UserModel", "ArticleModel").Create(&m).Error; err != nil {
		t.Fatalf("插入浏览历史 %d 失败: %v", id, err)
	}
}

// ReviewTaskSpec 描述要插入的审核任务行。
type ReviewTaskSpec struct {
	ID                   uint64
	ArticleID            uint64
	AuthorID             uint64
	ArticleTitle         string
	AuthorName           string
	ArticlePublishStatus ArticleStatus
	Stage                ReviewTaskStage
	Source               ReviewTaskSource
	Status               ReviewTaskStatus
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// InsertReviewTask 插入一条审核任务。
func InsertReviewTask(t *testing.T, db *gorm.DB, spec ReviewTaskSpec) {
	t.Helper()
	m := models.ArticleReviewTaskModel{
		Model:                models.Model{ID: ctype.ID(spec.ID), CreatedAt: spec.CreatedAt, UpdatedAt: spec.UpdatedAt},
		ArticleID:            ctype.ID(spec.ArticleID),
		AuthorID:             ctype.ID(spec.AuthorID),
		ArticleTitle:         spec.ArticleTitle,
		AuthorName:           spec.AuthorName,
		ArticlePublishStatus: spec.ArticlePublishStatus,
		Stage:                spec.Stage,
		Source:               spec.Source,
		Status:               spec.Status,
	}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("插入审核任务 %d 失败: %v", spec.ID, err)
	}
}
