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
	"fmt"
	"sort"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/kernel/enum/article_review_enum"
	"myblogx/internal/platform/cdc/outboxrelay"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// 审核任务的三组枚举（定义在 kernel，业务模块可自由引用）。
type (
	ReviewTaskStage  = article_review_enum.Stage
	ReviewTaskSource = article_review_enum.Source
	ReviewTaskStatus = article_review_enum.Status
)

// 审核任务取值。
const (
	ReviewStageManual    = article_review_enum.StageManual
	ReviewSourceCreate   = article_review_enum.SourceCreate
	ReviewSourceEdit     = article_review_enum.SourceEdit
	ReviewSourceResubmit = article_review_enum.SourceResubmit
	ReviewStatusPending  = article_review_enum.StatusPending
	ReviewStatusApproved = article_review_enum.StatusApproved
	ReviewStatusRejected = article_review_enum.StatusRejected
	ReviewStatusCanceled = article_review_enum.StatusCanceled
)

// outbox 状态取值。
//
// 直接引用中继的常量而不是再抄一份：状态机只有中继一个使用方，
// 它改了口径（比如新增 "retrying"）时这里自动跟上。
const (
	OutboxStatusPending    = outboxrelay.OutboxStatusPending
	OutboxStatusProcessing = outboxrelay.OutboxStatusProcessing
	OutboxStatusDone       = outboxrelay.OutboxStatusDone
	OutboxStatusFailed     = outboxrelay.OutboxStatusFailed
	OutboxStatusDead       = outboxrelay.OutboxStatusDead
)

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
	//
	// 含**两种**置顶：作者置顶（user_top_article_models）与热门置顶运营位
	// （article_hot_top_models）。两者是两套数据，都归"读/写关系"这组。
	UserReading Group = "user_reading"
	// Comment 是评论表。
	Comment Group = "comment"
	// Chat 是会话与消息表。
	Chat Group = "chat"
	// Outbox 是事务型发件箱。
	Outbox Group = "outbox"
	// ImageAssets 是图片本体与引用关系。
	ImageAssets Group = "image_assets"
	// Site 是站点配置与轮播位。
	Site Group = "site"
	// Social 是关注关系。
	Social Group = "social"
	// DeadLetter 是 CDC 死信。
	DeadLetter Group = "dead_letter"
	// AIProvider 是启用中的模型配置（AI 调用方读的那张表）。
	AIProvider Group = "ai_provider"
	// All 是全部表（仅用于结构一致性契约测试）。
	All Group = "all"
)

// groupModels 是每个表组对应的行结构，**组内顺序即建表顺序**。
var groupModels = map[Group][]any{
	UserCore: {
		&UserRow{},
		&UserConfRow{},
		&UserStatRow{},
	},
	UserSession: {
		&UserSessionRow{},
	},
	// ArticleCore 含 ArticleTagRow，因此 article_tag_models 会带上
	// id / created_at / updated_at / deleted_at 四列（不只是 (article_id, tag_id)）。
	//
	// 这是刻意的：标签仓储（article/adapter/mysql 的 ReplaceTags）用
	// ArticleTagModel 结构体读写这张表，表里没有 id 列会报 Unknown column 'id'。
	// 代价是插入时必须显式给 id —— 该列 NOT NULL 且无默认值，
	// 只写两列会报 Error 1364。见 InsertArticleTag。
	ArticleCore: {
		&CategoryRow{},
		&TagRow{},
		&ArticleRow{},
		&ArticleTagRow{},
		&ArticleDiggRow{},
	},
	ArticleReview: {
		&ArticleReviewTaskRow{},
		&ArticleReviewLogRow{},
	},
	Favorite: {
		&FavoriteRow{},
		&UserArticleFavorRow{},
	},
	UserReading: {
		&UserTopArticleRow{},
		&ArticleHotTopRow{},
		&UserArticleViewHistoryRow{},
	},
	Comment: {
		&CommentRow{},
	},
	// Chat 里消息表必须排在会话表**之前**。
	//
	// ChatMsgModel.SessionList 是一条 hasMany，它会在 chat_session_models 上
	// 建一条指向 chat_msg_models.session_id 的外键。先建会话表的话，
	// 被引用的表还不存在，MySQL 直接报 Error 1824。
	Chat: {
		&ChatMsgRow{},
		&ChatSessionRow{},
		&ChatMsgUserStateRow{},
	},
	Outbox: {
		&OutboxEventRow{},
	},
	ImageAssets: {
		&ImageRow{},
		&ImageRefRow{},
	},
	Site: {
		&BannerRow{},
		&BannerCropRow{},
		&RuntimeConfigRow{},
	},
	Social: {
		&UserFollowRow{},
	},
	DeadLetter: {
		&CdcDeadLetterRow{},
	},
	AIProvider: {
		&AIProviderRow{},
	},
	// All 是**全部 35 张表**，只用于"schema.sql 与自持行是否一致"的契约测试。
	//
	// 为什么不复用上面那些组拼起来：拼装顺序一变，这个测试就变成
	// "测拼装顺序"而不是"测结构一致性"。这里写死一份，顺序沿用当年
	// flag_db 的 AutoMigrate 顺序（被引用的表在前），AutoMigrate 建外键不会撞 1824。
	All: {
		&UserRow{},
		&UserConfRow{},
		&UserStatRow{},
		&UserSessionRow{},
		&UserViewDailyRow{},
		&CategoryRow{},
		&TagRow{},
		&ArticleRow{},
		&ArticleAIScoreRecordRow{},
		&ArticleReviewTaskRow{},
		&ArticleReviewLogRow{},
		&ArticleTagRow{},
		&ArticleDiggRow{},
		&FavoriteRow{},
		&UserArticleFavorRow{},
		&UserArticleViewHistoryRow{},
		&UserTopArticleRow{},
		&ArticleHotTopRow{},
		&ImageRow{},
		&ImageRefRow{},
		&RuntimeConfigRow{},
		&CommentRow{},
		&BannerRow{},
		&GlobalNotifRow{},
		&CommentDiggRow{},
		&ArticleMessageRow{},
		&UserGlobalNotifRow{},
		&UserFollowRow{},
		&ChatMsgRow{},
		&ChatSessionRow{},
		&ChatMsgUserStateRow{},
		&CdcDeadLetterRow{},
		&OutboxEventRow{},
		&AIProviderRow{},
		&BannerCropRow{},
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
	u := UserRow{}
	u.ID = ctype.NewID(spec.ID)
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

// UserStatSpec 描述要插入的用户统计行。
type UserStatSpec struct {
	UserID              uint64
	ViewCount           int
	FansCount           int
	FollowCount         int
	ArticleVisitedCount int
	ArticleCount        int
}

// InsertUserStat 写入一行用户统计（存在则覆盖）。
//
// 为什么用 upsert 而不是 Create：InsertUser 会通过 GORM 关联顺带建出一行
// 零值统计，直接 Create 会主键冲突。凡是断言统计字段的测试都必须显式写入
// 非零值——否则读到的全是零值，"字段没接上"和"统计真的是 0"无法区分。
func InsertUserStat(t *testing.T, db *gorm.DB, spec UserStatSpec) {
	t.Helper()
	m := UserStatRow{
		UserID:              ctype.NewID(spec.UserID),
		ViewCount:           spec.ViewCount,
		FansCount:           spec.FansCount,
		FollowCount:         spec.FollowCount,
		ArticleVisitedCount: spec.ArticleVisitedCount,
		ArticleCount:        spec.ArticleCount,
	}
	err := db.Omit("UserModel").Clauses(clause.OnConflict{UpdateAll: true}).Create(&m).Error
	if err != nil {
		t.Fatalf("写入用户统计 %d 失败: %v", spec.UserID, err)
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
	m := ArticleRow{
		BaseRow: BaseRow{
			ID:        ctype.NewID(spec.ID),
			CreatedAt: spec.CreatedAt,
			UpdatedAt: spec.UpdatedAt,
		},
		Title:            spec.Title,
		Abstract:         spec.Abstract,
		Content:          spec.Content,
		CategoryID:       spec.CategoryID,
		AuthorID:         ctype.NewID(spec.AuthorID),
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
	m := CategoryRow{
		BaseRow: BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		Title:   title,
		UserID:  ctype.NewID(userID),
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
	m := TagRow{
		BaseRow:      BaseRow{ID: ctype.NewID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		Title:        spec.Title,
		Sort:         spec.Sort,
		ArticleCount: spec.ArticleCount,
		IsEnabled:    spec.Enabled,
		CreatedBy:    ctype.NewID(spec.CreatedBy),
	}
	if err := db.Omit("ArticleList").Create(&m).Error; err != nil {
		t.Fatalf("插入标签 %d 失败: %v", spec.ID, err)
	}
	if !spec.Enabled {
		// 见 TagSpec.Enabled 的说明：默认值会把 false 顶回来。
		if err := db.Model(&TagRow{}).Where("id = ?", spec.ID).
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
	m := FavoriteRow{
		BaseRow:      BaseRow{ID: ctype.NewID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		UserID:       ctype.NewID(spec.UserID),
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
	m := UserArticleFavorRow{
		BaseRow:               BaseRow{ID: ctype.NewID(spec.ID), CreatedAt: spec.At, UpdatedAt: spec.At},
		ArticleID:             ctype.NewID(spec.ArticleID),
		UserID:                ctype.NewID(spec.UserID),
		FavorID:               ctype.NewID(spec.FavorID),
		ArticleTitle:          spec.ArticleTitle,
		ArticleAbstract:       spec.ArticleAbstract,
		ArticleStatus:         spec.ArticleStatus,
		ArticleAuthorID:       ctype.NewID(spec.ArticleAuthorID),
		ArticleAuthorNickname: spec.ArticleAuthorNickname,
	}
	if err := db.Omit("UserModel", "ArticleModel", "FavoriteModel").Create(&m).Error; err != nil {
		t.Fatalf("插入收藏关系 %d 失败: %v", spec.ArticleID, err)
	}
}

// ArticleDiggSpec 描述要插入的点赞关系行。
//
// SoftDeleted 用于造"线上存量残行"：唯一索引 (article_id, user_id) 不含
// deleted_at，取消点赞留下的软删行仍占着键。测试要能造出这种行，
// 才能验证"老用户再次点赞不受历史残行影响"（#96 AC 第 3 条）。
type ArticleDiggSpec struct {
	ID          uint64
	ArticleID   uint64
	UserID      uint64
	SoftDeleted bool
	At          time.Time
}

// InsertArticleDigg 插入一条点赞关系行。
//
// deleted_at 只能靠插入后 UPDATE 写入：GORM 会把它当普通列处理，
// 而 gorm.DeletedAt 是 sql.NullTime 的包装，直接给零值以外的初值
// 在 Create 时不可靠。这里显式 UPDATE 一次，语义清楚且不依赖 GORM 行为。
func InsertArticleDigg(t *testing.T, db *gorm.DB, spec ArticleDiggSpec) {
	t.Helper()
	at := spec.At
	if at.IsZero() {
		at = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	m := ArticleDiggRow{
		BaseRow:   BaseRow{ID: ctype.NewID(spec.ID), CreatedAt: at, UpdatedAt: at},
		ArticleID: ctype.NewID(spec.ArticleID),
		UserID:    ctype.NewID(spec.UserID),
	}
	if err := db.Omit("ArticleModel", "UserModel").Create(&m).Error; err != nil {
		t.Fatalf("插入点赞关系 %d 失败: %v", spec.ID, err)
	}
	if !spec.SoftDeleted {
		return
	}
	// 软删：填上 deleted_at。用 UPDATE 而不是 Delete，
	// 是为了让"这行是历史残行"这件事在测试里显式可读。
	if err := db.Model(&ArticleDiggRow{}).
		Where("id = ?", ctype.NewID(spec.ID)).
		Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删点赞关系 %d 失败: %v", spec.ID, err)
	}
}

// InsertUserTopArticle 插入一条置顶记录。
func InsertUserTopArticle(t *testing.T, db *gorm.DB, id, userID, articleID uint64, at time.Time) {
	t.Helper()
	m := UserTopArticleRow{
		BaseRow:   BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		UserID:    ctype.NewID(userID),
		ArticleID: ctype.NewID(articleID),
	}
	if err := db.Omit("UserModel", "ArticleModel").Create(&m).Error; err != nil {
		t.Fatalf("插入置顶 %d 失败: %v", id, err)
	}
}

// InsertArticleHotTop 插入一条热门置顶（首页运营位）。
//
// 与 InsertUserTopArticle（作者置顶）是两套数据：这里只有 article_id + 序号。
func InsertArticleHotTop(t *testing.T, db *gorm.DB, id, articleID uint64, sortOrder int, at time.Time) {
	t.Helper()
	m := ArticleHotTopRow{
		BaseRow:   BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		ArticleID: ctype.NewID(articleID),
		SortOrder: sortOrder,
	}
	if err := db.Omit("ArticleModel").Create(&m).Error; err != nil {
		t.Fatalf("插入热门置顶 %d 失败: %v", id, err)
	}
}

// InsertViewHistory 插入一条浏览历史。
func InsertViewHistory(t *testing.T, db *gorm.DB, id, userID, articleID uint64, at time.Time) {
	t.Helper()
	m := UserArticleViewHistoryRow{
		BaseRow:   BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		UserID:    ctype.NewID(userID),
		ArticleID: ctype.NewID(articleID),
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
	m := ArticleReviewTaskRow{
		BaseRow:              BaseRow{ID: ctype.NewID(spec.ID), CreatedAt: spec.CreatedAt, UpdatedAt: spec.UpdatedAt},
		ArticleID:            ctype.NewID(spec.ArticleID),
		AuthorID:             ctype.NewID(spec.AuthorID),
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

// ColumnsDiff 比对"row 结构体推导出的列"与"真实表列"。
//
// 返回 (结构体有但表没有, 表有但结构体没映射)。两者都为空才算一致。
//
// # 为什么必须有这条契约
//
// 「零 DDL」的真实含义是：**MySQL 的列名是权威**，ES 投影靠 CDC 解析 binlog
// 的列名来认字段。所以重构时把 row 结构体搬个位置没问题，
// 但改一个字段名/少一个字段，就等于改了列名 —— 而这类错误几乎不会报错：
//
//   - 结构体少一个字段 → GORM 的 INSERT/UPDATE 不会写那一列，
//     投影侧读到的是上一版的旧值（readmodel 的"快照同步"全是这种写法）；
//   - 结构体多一个字段 → SELECT 直接报 Unknown column，反而容易发现。
//
// 也就是说**沉默的、危险的是"漏字段"**。两个方向都返回，正是为了把它揪出来。
func ColumnsDiff(db *gorm.DB, row any) (missing, extra []string, err error) {
	stmt := &gorm.Statement{DB: db}
	if err = stmt.Parse(row); err != nil {
		return nil, nil, err
	}
	table := stmt.Schema.Table

	want := make(map[string]bool, len(stmt.Schema.Fields))
	for _, f := range stmt.Schema.Fields {
		if f.DBName == "" || f.IgnoreMigration {
			continue
		}
		want[f.DBName] = true
	}

	var got []string
	if err = db.Raw(
		"SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?",
		table,
	).Scan(&got).Error; err != nil {
		return nil, nil, err
	}
	if len(got) == 0 {
		return nil, nil, fmt.Errorf("表 %s 不存在或没有列（先用 schema.Migrate 建表）", table)
	}

	have := make(map[string]bool, len(got))
	for _, c := range got {
		have[c] = true
	}
	for col := range want {
		if !have[col] {
			missing = append(missing, col)
		}
	}
	for col := range have {
		if !want[col] {
			extra = append(extra, col)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra, nil
}

// AssertColumns 把 ColumnsDiff 的差异变成测试失败。
func AssertColumns(t *testing.T, db *gorm.DB, row any) {
	t.Helper()

	missing, extra, err := ColumnsDiff(db, row)
	if err != nil {
		t.Fatalf("列名契约比对失败: %v", err)
	}
	if len(missing) > 0 {
		t.Errorf("结构体有、表里没有的列 %v（查询会直接报 Unknown column）", missing)
	}
	if len(extra) > 0 {
		t.Errorf("表里有、结构体没映射的列 %v（该列会被静默忽略：读不到、也写不进去）", extra)
	}
}
