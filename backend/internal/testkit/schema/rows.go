// 本文件由 M6 的一次性生成器从 models 行结构复制而来（零手改）。
//
// # 为什么是"复制"而不是"引用"
//
// M6 要删掉 models 包。测试的建表与造数据仍需要行结构，所以必须有一份新的持有者。
// testkit 不是业务模块（archcheck 的 moduleOf 对它返回空），是这些结构合法的落点。
//
// # 复制的三条纪律
//
//  1. **字段顺序与 gorm tag 一字不改** —— 表结构由它们决定，改一个 tag 就可能改列；
//  2. **显式 TableName** —— 类型改名为 XxxRow 后 GORM 会推导出 `xxx_rows`，
//     与真实表名不符，AutoMigrate 会建出一张空的新表；
//  3. **钩子一并复制** —— BeforeCreate 生成雪花主键、UserRow.AfterCreate 建
//     user_conf/user_stat 默认行，漏了测试行为就变了。
//
// 与真实表的偏差由 columns_integration_test.go 的列名契约兜住。

package schema

import (
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/kernel/enum/article_review_enum"
	"myblogx/internal/kernel/enum/chat_msg_enum"
	"myblogx/internal/kernel/enum/global_notif_enum"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/kernel/enum/message_enum"
	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BaseRow 是所有行的公共列（主键 + 三个时间列）。
//
// 与 internal/platform/mysql.BaseRow 同构：主键是雪花值、数据库没有默认值，
// 所以必须靠 BeforeCreate 在代码里生成。
type BaseRow struct {
	ID        ctype.ID       `gorm:"primaryKey;autoIncrement:false" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at"`
}

// BeforeCreate 生成雪花主键（只在主键为空时生成，让测试能显式指定 ID）。
func (b *BaseRow) BeforeCreate(*gorm.DB) error {
	if !b.ID.IsZero() {
		return nil
	}
	next, err := idgen.Next()
	if err != nil {
		return err
	}
	b.ID = next
	return nil
}

// ArticleAIScoreRecordRow 对应表 article_ai_score_record_models。
type ArticleAIScoreRecordRow struct {
	BaseRow
	ArticleID                ctype.ID   `gorm:"index;not null" json:"article_id"`
	UserID                   ctype.ID   `gorm:"index;not null" json:"user_id"`
	TitleSnapshot            string     `gorm:"size:256;not null" json:"title_snapshot"`
	ContentHash              string     `gorm:"size:64;index;not null" json:"content_hash"`
	ContentLength            int        `gorm:"default:0" json:"content_length"`
	ArticleUpdatedAtSnapshot *time.Time `json:"article_updated_at_snapshot"`
	AITotalScore             int        `gorm:"default:0" json:"ai_total_score"`
	TotalScore               int        `gorm:"default:0;index" json:"total_score"`
	ScoreLevel               string     `gorm:"size:32" json:"score_level"`
	ArticleType              string     `gorm:"size:32" json:"article_type"`
	DimensionsJSON           string     `gorm:"type:longtext;not null" json:"dimensions_json"`
	MainIssuesJSON           string     `gorm:"type:longtext;not null" json:"main_issues_json"`
	OverallComment           string     `gorm:"type:longtext" json:"overall_comment"`
	Provider                 string     `gorm:"size:32" json:"provider"`
	ModelName                string     `gorm:"size:64" json:"model_name"`
	PromptVersion            string     `gorm:"size:32" json:"prompt_version"`
}

// TableName 固定表名。
func (ArticleAIScoreRecordRow) TableName() string { return "article_ai_score_record_models" }

// ArticleDiggRow 对应表 article_digg_models。
type ArticleDiggRow struct {
	BaseRow
	ArticleID    ctype.ID   `gorm:"uniqueIndex:uk_article_digg,priority:1" json:"article_id"`
	UserID       ctype.ID   `gorm:"uniqueIndex:uk_article_digg,priority:2" json:"user_id"`
	ArticleModel ArticleRow `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
	UserModel    UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
}

// TableName 固定表名。
func (ArticleDiggRow) TableName() string { return "article_digg_models" }

// ArticleRow 对应表 article_models。
type ArticleRow struct {
	BaseRow
	Title            string                       `gorm:"size:256" json:"title"`
	Abstract         string                       `gorm:"size:256" json:"abstract"`
	Content          string                       `gorm:"type:longtext" json:"content"`
	ContentHead      string                       `gorm:"-" json:"content_head,omitempty"` // ES 冗余字段，保存去除 Markdown 格式后的正文前 150 字
	CategoryID       *ctype.ID                    `gorm:"index" json:"category_id"`
	Cover            string                       `gorm:"size:256" json:"cover"`
	AuthorID         ctype.ID                     `gorm:"index" json:"author_id"`
	ViewCount        int                          `gorm:"default:0" json:"view_count"`
	DiggCount        int                          `gorm:"default:0" json:"digg_count"`
	CommentCount     int                          `gorm:"default:0" json:"comment_count"`
	FavorCount       int                          `gorm:"default:0" json:"favor_count"`
	CommentsToggle   bool                         `gorm:"default:true" json:"comments_toggle"`
	PublishStatus    enum.ArticleStatus           `gorm:"default:0;index" json:"publish_status"`
	VisibilityStatus enum.ArticleVisibilityStatus `gorm:"type:varchar(32);default:'visible';index" json:"visibility_status"`
	SubmittedAt      *time.Time                   `json:"submitted_at"`
	ReviewedAt       *time.Time                   `json:"reviewed_at"`
	ReviewedBy       *ctype.ID                    `gorm:"index" json:"reviewed_by"`
	UserModel        UserRow                      `gorm:"foreignKey:AuthorID;references:ID" json:"-"`
	CategoryModel    *CategoryRow                 `gorm:"foreignKey:CategoryID;references:ID" json:"-"`
	Tags             []TagRow                     `gorm:"many2many:article_tag_models;joinForeignKey:ArticleID;joinReferences:TagID" json:"tags"`
}

// TableName 固定表名。
func (ArticleRow) TableName() string { return "article_models" }

// ArticleReviewTaskRow 对应表 article_review_task_models。
type ArticleReviewTaskRow struct {
	BaseRow
	ArticleID            ctype.ID                   `gorm:"index;not null" json:"article_id"`
	AuthorID             ctype.ID                   `gorm:"index;not null" json:"author_id"`
	ArticleTitle         string                     `gorm:"size:256;not null" json:"article_title"`
	AuthorName           string                     `gorm:"size:32;not null" json:"author_name"`
	ArticlePublishStatus enum.ArticleStatus         `gorm:"default:0;index" json:"publish_status"`
	Stage                article_review_enum.Stage  `gorm:"type:varchar(16);not null;default:'manual'" json:"stage"`
	Source               article_review_enum.Source `gorm:"type:varchar(16);not null" json:"source"`
	Status               article_review_enum.Status `gorm:"type:varchar(16);not null;default:'pending';index" json:"status"`
	Reason               string                     `gorm:"size:500" json:"reason"`
	ReviewedAt           *time.Time                 `json:"reviewed_at"`
	ReviewedBy           *ctype.ID                  `gorm:"index" json:"reviewed_by"`
}

// TableName 固定表名。
func (ArticleReviewTaskRow) TableName() string { return "article_review_task_models" }

// ArticleReviewLogRow 对应表 article_review_log_models。
type ArticleReviewLogRow struct {
	BaseRow
	TaskID     ctype.ID `gorm:"index;not null" json:"task_id"`
	ArticleID  ctype.ID `gorm:"index;not null" json:"article_id"`
	OperatorID ctype.ID `gorm:"index;not null" json:"operator_id"`
	Action     string   `gorm:"size:32;not null" json:"action"`
	FromStatus string   `gorm:"size:32" json:"from_status"`
	ToStatus   string   `gorm:"size:32" json:"to_status"`
	Reason     string   `gorm:"size:500" json:"reason"`
}

// TableName 固定表名。
func (ArticleReviewLogRow) TableName() string { return "article_review_log_models" }

// ArticleTagRow 对应表 article_tag_models。
type ArticleTagRow struct {
	BaseRow
	ArticleID ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:1;index:idx_article_tag_article,priority:1" json:"article_id"`
	TagID     ctype.ID `gorm:"uniqueIndex:uk_article_tag,priority:2;index:idx_article_tag_tag,priority:1" json:"tag_id"`
	Article   ArticleRow
	Tag       TagRow
}

// TableName 固定表名。
func (ArticleTagRow) TableName() string { return "article_tag_models" }

// BannerRow 对应表 banner_models。
type BannerRow struct {
	BaseRow
	Show         bool      `json:"show"`                    // 是否显示
	Cover        string    `json:"cover"`                   // 封面图片链接
	CoverImageID *ctype.ID `gorm:"-" json:"cover_image_id"` // 封面图片 ID，用于前端编辑回填
	Href         string    `json:"href"`                    // 跳转链接
}

// TableName 固定表名。
func (BannerRow) TableName() string { return "banner_models" }

// CategoryRow 对应表 category_models。
type CategoryRow struct {
	BaseRow
	Title       string       `gorm:"size:256;uniqueIndex:uk_category_user_title,priority:2" json:"title"`
	UserID      ctype.ID     `gorm:"uniqueIndex:uk_category_user_title,priority:1;index" json:"user_id"`
	UserModel   UserRow      `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleList []ArticleRow `gorm:"foreignKey:CategoryID" json:"-"`
}

// TableName 固定表名。
func (CategoryRow) TableName() string { return "category_models" }

// CdcDeadLetterRow 对应表 cdc_dead_letter。
type CdcDeadLetterRow struct {
	ID ctype.ID `gorm:"primaryKey;autoIncrement:false" json:"id"`

	Stream      string `gorm:"size:32;not null;index:idx_cdc_dlq_stream_status_created,priority:1;uniqueIndex:uk_cdc_dlq_stream_job,priority:1" json:"stream"`
	CdcJobID    string `gorm:"size:255;not null;uniqueIndex:uk_cdc_dlq_stream_job,priority:2;index:idx_cdc_dlq_job_id" json:"cdc_job_id"`
	SourceTable string `gorm:"size:128;not null" json:"source_table"`
	Action      string `gorm:"size:32;not null" json:"action"`
	TargetKey   string `gorm:"size:255" json:"target_key"`
	PayloadJSON string `gorm:"type:longtext;not null" json:"payload_json"`
	RetryCount  int    `gorm:"default:0;not null" json:"retry_count"`
	Status      string `gorm:"size:32;not null;index:idx_cdc_dlq_stream_status_created,priority:2" json:"status"`
	ErrorCode   string `gorm:"size:64" json:"error_code"`
	ErrorMsg    string `gorm:"type:text" json:"error_msg"`

	CreatedAt time.Time `gorm:"index:idx_cdc_dlq_stream_status_created,priority:3" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (m *CdcDeadLetterRow) BeforeCreate(_ *gorm.DB) (err error) {
	if !m.ID.IsZero() {
		return nil
	}
	m.ID, err = idgen.Next()
	return err
}

// TableName 固定表名。
func (CdcDeadLetterRow) TableName() string { return "cdc_dead_letter" }

// ChatSessionRow 对应表 chat_session_models。
type ChatSessionRow struct {
	BaseRow
	SessionID        string     `gorm:"size:64;not null;index:idx_chat_session_id" json:"session_id"`
	UserID           ctype.ID   `gorm:"not null;uniqueIndex:uk_chat_session_user_receiver,priority:1;index:idx_chat_session_user_time,priority:1" json:"user_id"`
	ReceiverID       ctype.ID   `gorm:"not null;uniqueIndex:uk_chat_session_user_receiver,priority:2" json:"receiver_id"`
	ReceiverNickname string     `gorm:"size:64" json:"receiver_nickname"`
	ReceiverAvatar   string     `gorm:"size:256" json:"receiver_avatar"`
	LastMsgID        ctype.ID   `json:"last_msg_id"`
	LastMsgContent   string     `json:"last_msg_content"`
	LastMsgTime      *time.Time `gorm:"index:idx_chat_session_user_time,priority:2,sort:desc" json:"last_msg_time"`
	ClearBeforeMsgID ctype.ID   `json:"clear_before_msg_id"`
	UnreadCount      int        `json:"unread_count"`
	UserModel        UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ReceiverModel    UserRow    `gorm:"foreignKey:ReceiverID;references:ID" json:"-"`
	IsTop            bool       `json:"is_top"`  // 是否置顶
	IsMute           bool       `json:"is_mute"` // 是否静音
}

// TableName 固定表名。
func (ChatSessionRow) TableName() string { return "chat_session_models" }

// ChatMsgRow 对应表 chat_msg_models。
type ChatMsgRow struct {
	BaseRow
	SenderID   ctype.ID                `json:"sender_id"`
	ReceiverID ctype.ID                `json:"receiver_id"`
	SessionID  string                  `gorm:"size:64;not null;index:idx_chat_msg_session_time,priority:1" json:"session_id"`
	Content    string                  `json:"content"`
	SendTime   time.Time               `gorm:"index:idx_chat_msg_session_time,priority:2,sort:desc" json:"send_time"`
	ReadAt     *time.Time              `json:"read_at"`
	MsgStatus  chat_msg_enum.MsgStatus `json:"msg_status"`
	MsgType    chat_msg_enum.MsgType   `json:"msg_type"`
	// 这里原来有一个 SessionList []ChatSessionModel（hasMany，foreignKey:SessionID）。
	// 它是个纯粹的建模错误，已删除：
	//   1. 全仓零使用 —— 从来没有人读"某条消息属于哪些会话"；
	//   2. GORM 会据此在 chat_session_models.session_id 上建一条指向
	//      chat_msg_models.session_id 的外键，而发送消息的顺序是
	//      "先建会话行、再插消息行"，这条外键让第一步必然失败
	//      （Error 1452）；删消息也会被它拦住；
	//   3. chat_msg_models.session_id 不是唯一键，本来就不该被引用。
	// 见 docs/architecture/09-M6决策清单.md 问题 2。
}

// TableName 固定表名。
func (ChatMsgRow) TableName() string { return "chat_msg_models" }

// ChatMsgUserStateRow 对应表 chat_msg_user_state_models。
type ChatMsgUserStateRow struct {
	BaseRow
	MsgID     ctype.ID `gorm:"not null;uniqueIndex:uk_chat_msg_user_state,priority:1;index:idx_chat_msg_user_msg,priority:2;index:idx_chat_msg_user_session_deleted,priority:4" json:"msg_id"`
	UserID    ctype.ID `gorm:"not null;uniqueIndex:uk_chat_msg_user_state,priority:2;index:idx_chat_msg_user_msg,priority:1;index:idx_chat_msg_user_session_deleted,priority:1" json:"user_id"`
	SessionID string   `gorm:"size:64;not null;index:idx_chat_msg_user_session_deleted,priority:2" json:"session_id"`
}

// TableName 固定表名。
func (ChatMsgUserStateRow) TableName() string { return "chat_msg_user_state_models" }

// CommentRow 对应表 comment_models。
type CommentRow struct {
	BaseRow
	Content           string             `json:"content" gorm:"type:text;not null"`
	UserID            ctype.ID           `json:"user_id"`
	UserNickname      string             `json:"user_nickname" gorm:"size:64"`
	UserAvatar        string             `json:"user_avatar" gorm:"size:256"`
	UserModel         UserRow            `json:"user_model" gorm:"foreignKey:UserID;references:ID"`
	ArticleID         ctype.ID           `json:"article_id" gorm:"not null;index:idx_article_root"`
	ReplyId           ctype.ID           `json:"reply_id" gorm:"default:0"` // 回复的评论id，0表示一级评论
	ReplyUserID       ctype.ID           `json:"reply_user_id" gorm:"default:0"`
	ReplyUserNickname string             `json:"reply_user_nickname" gorm:"size:64"`
	RootID            ctype.ID           `json:"root_id" gorm:"default:0;index:idx_article_root"` // 根评论ID，0表示本身就是一级评论
	DiggCount         int                `json:"digg_count" gorm:"default:0"`                     // 点赞数
	ReplyCount        int                `json:"reply_count" gorm:"default:0"`
	Status            enum.CommentStatus `gorm:"default:0" json:"status"`
	ArticleModel      ArticleRow         `json:"article_model" gorm:"foreignKey:ArticleID;references:ID"`
	ParentModel       *CommentRow        `json:"parent_model" gorm:"foreignKey:ReplyId;references:ID"`
}

// TableName 固定表名。
func (CommentRow) TableName() string { return "comment_models" }

// CommentDiggRow 对应表 comment_digg_models。
type CommentDiggRow struct {
	BaseRow
	CommentID    ctype.ID   `gorm:"uniqueIndex:uk_comment_digg,priority:1" json:"comment_id"`
	UserID       ctype.ID   `gorm:"uniqueIndex:uk_comment_digg,priority:2" json:"user_id"`
	UserModel    UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	CommentModel CommentRow `gorm:"foreignKey:CommentID;references:ID" json:"-"`
}

// TableName 固定表名。
func (CommentDiggRow) TableName() string { return "comment_digg_models" }

// FavoriteRow 对应表 favorite_models。
type FavoriteRow struct {
	BaseRow
	UserID        ctype.ID              `gorm:"uniqueIndex:uk_favorite_user_title,priority:1;index" json:"user_id"`
	Title         string                `gorm:"size:32;uniqueIndex:uk_favorite_user_title,priority:2" json:"title"`
	Cover         string                `gorm:"size:256" json:"cover"`
	Abstract      string                `gorm:"size:256" json:"abstract"`
	IsDefault     bool                  `gorm:"default:false" json:"is_default"`
	ArticleCount  int                   `gorm:"default:0" json:"article_count"`
	OwnerNickname string                `gorm:"size:64" json:"owner_nickname"`
	OwnerAvatar   string                `gorm:"size:256" json:"owner_avatar"`
	UserModel     UserRow               `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleList   []UserArticleFavorRow `gorm:"foreignKey:FavorID" json:"-"`
}

// TableName 固定表名。
func (FavoriteRow) TableName() string { return "favorite_models" }

// GlobalNotifRow 对应表 global_notif_models。
type GlobalNotifRow struct {
	BaseRow
	UserVisibleRule global_notif_enum.Type `json:"user_visible_rule"`
	ExpireTime      time.Time              `json:"expire_time"`             // 通知过期
	ActionUser      ctype.ID               `json:"action_user"`             // 操作人
	Title           string                 `gorm:"size:64" json:"title"`    // 通知标题
	Icon            string                 `gorm:"size:64" json:"icon"`     // 通知图标
	Content         string                 `gorm:"size:128" json:"content"` // 通知内容
	Href            string                 `gorm:"size:256" json:"herf"`    // 通知链接
}

// TableName 固定表名。
func (GlobalNotifRow) TableName() string { return "global_notif_models" }

// ImageRow 对应表 image_models。
type ImageRow struct {
	BaseRow
	UserID    ctype.ID           `gorm:"index" json:"user_id"`
	Provider  enum.ImageProvider `gorm:"size:16;default:qiniu" json:"provider"`
	Bucket    string             `gorm:"size:64" json:"bucket"`
	ObjectKey string             `gorm:"size:255;uniqueIndex:uk_image_object_key" json:"object_key"`
	FileName  string             `gorm:"size:255" json:"file_name"`
	URL       string             `gorm:"size:512" json:"url"`
	MimeType  string             `gorm:"size:128" json:"mime_type"`
	Size      int64              `gorm:"default:0" json:"size"`
	Width     int                `gorm:"default:0" json:"width"`
	Height    int                `gorm:"default:0" json:"height"`
	Hash      string             `gorm:"size:64;uniqueIndex:uk_image_hash" json:"hash"`
	Status    enum.ImageStatus   `gorm:"default:1" json:"status"`
}

// TableName 固定表名。
func (ImageRow) TableName() string { return "image_models" }

// ImageRefRow 对应表 image_ref_models。
type ImageRefRow struct {
	BaseRow
	ImageID    ctype.ID                `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:5" json:"image_id"`
	RefType    image_ref_enum.RefType  `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:1" json:"ref_type"`
	OwnerID    ctype.ID                `gorm:"index;uniqueIndex:uk_image_ref_owner_field_pos,priority:2" json:"owner_id"`
	Field      image_ref_enum.RefField `gorm:"index;not null;uniqueIndex:uk_image_ref_owner_field_pos,priority:3" json:"field"`
	Position   int                     `gorm:"default:0;uniqueIndex:uk_image_ref_owner_field_pos,priority:4" json:"position"`
	ImageModel ImageRow                `gorm:"foreignKey:ImageID;references:ID" json:"image_model"`
}

// TableName 固定表名。
func (ImageRefRow) TableName() string { return "image_ref_models" }

// ArticleMessageRow 对应表 article_message_models。
type ArticleMessageRow struct {
	BaseRow
	Type message_enum.Type `json:"type"`

	ReceiverID         ctype.ID  `gorm:"index:idx_msg_receiver_read_created,priority:1" json:"receiver_id"`
	ActionUserID       *ctype.ID `gorm:"index" json:"action_user_id"`
	ActionUserNickname *string   `json:"action_user_nickname"`
	ActionUserAvatar   *string   `json:"action_user_avatar"`

	Content string `gorm:"type:text" json:"content"`

	// 记录触发的业务对象
	ArticleID    ctype.ID `json:"article_id"`
	CommentID    ctype.ID `json:"comment_id"`
	ArticleTitle string   `json:"article_title"`

	// 额外提示的链接
	LinkTitle string `gorm:"size:128" json:"link_title"`
	LinkHerf  string `gorm:"size:256" json:"link_herf"`

	// 是否已读
	IsRead bool       `gorm:"default:false;index:idx_msg_receiver_read_created,priority:2" json:"is_read"`
	ReadAt *time.Time `json:"read_at"`
}

// TableName 固定表名。
func (ArticleMessageRow) TableName() string { return "article_message_models" }

// OutboxEventRow 对应表 outbox_event_models。
type OutboxEventRow struct {
	BaseRow
	// EventName 事件名，如 "article.published"。
	EventName string `gorm:"size:64;not null;index" json:"event_name"`
	// AggregateType 聚合类型，如 "article"。
	AggregateType string `gorm:"size:64;not null" json:"aggregate_type"`
	// AggregateID 聚合 ID（如文章 ID），便于检索"某篇文章的事件"与人工排查。
	AggregateID uint64 `gorm:"not null;index" json:"aggregate_id"`
	// Payload 事件体（已序列化的 JSON 原文）。
	Payload string `gorm:"type:longtext" json:"payload"`
	// Status 见上面的状态机常量。
	Status string `gorm:"size:16;not null;index" json:"status"`
	// Attempts 已尝试次数，用于退避与判定是否耗尽重试。
	Attempts int `gorm:"not null;default:0" json:"attempts"`
	// AvailableAt 下次可领取的时间（失败后按退避后移）。
	AvailableAt time.Time `gorm:"not null;index" json:"available_at"`
	// LastError 最近一次失败原因，便于排查。
	LastError string `gorm:"size:512" json:"last_error"`
	// DispatchedAt 首次投递成功的时间；为空表示还没成功过。
	DispatchedAt *time.Time `json:"dispatched_at"`
}

// TableName 固定表名。
func (OutboxEventRow) TableName() string { return "outbox_event_models" }

// RuntimeSiteConfigRow 对应表 runtime_site_config_models。
type RuntimeSiteConfigRow struct {
	BaseRow
	Name     string `gorm:"size:32;uniqueIndex;not null" json:"name"`
	SiteJSON string `gorm:"type:longtext;not null" json:"site_json"`
	AIJSON   string `gorm:"type:longtext;not null" json:"ai_json"`
}

// TableName 固定表名。
func (RuntimeSiteConfigRow) TableName() string { return "runtime_site_config_models" }

// UserStatRow 对应表 user_stat_models。
type UserStatRow struct {
	UserID              ctype.ID `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	UserModel           UserRow  `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ViewCount           int      `gorm:"default:0" json:"view_count"`
	FansCount           int      `gorm:"default:0" json:"fans_count"`
	FollowCount         int      `gorm:"default:0" json:"follow_count"`
	ArticleVisitedCount int      `gorm:"default:0" json:"article_visited_count"`
	ArticleCount        int      `gorm:"default:0" json:"article_count"`
}

// TableName 固定表名。
func (UserStatRow) TableName() string { return "user_stat_models" }

// UserViewDailyRow 对应表 user_view_daily_models。
type UserViewDailyRow struct {
	BaseRow
	UserID       ctype.ID  `gorm:"uniqueIndex:uk_user_view_daily,priority:1" json:"user_id"`
	ViewerUserID ctype.ID  `gorm:"uniqueIndex:uk_user_view_daily,priority:2" json:"viewer_user_id"`
	ViewDate     time.Time `gorm:"type:date;uniqueIndex:uk_user_view_daily,priority:3" json:"view_date"`
}

// TableName 固定表名。
func (UserViewDailyRow) TableName() string { return "user_view_daily_models" }

// TagRow 对应表 tag_models。
type TagRow struct {
	BaseRow
	Title        string       `gorm:"size:64;uniqueIndex" json:"title"`
	Sort         int          `gorm:"default:0" json:"sort"`
	Description  string       `gorm:"size:255" json:"description"`
	ArticleCount int          `gorm:"default:0" json:"article_count"`
	IsEnabled    bool         `gorm:"default:true;index" json:"is_enabled"`
	CreatedBy    ctype.ID     `gorm:"index" json:"created_by"`
	ArticleList  []ArticleRow `gorm:"many2many:article_tag_models;joinForeignKey:TagID;joinReferences:ArticleID" json:"-"`
}

// TableName 固定表名。
func (TagRow) TableName() string { return "tag_models" }

// UserArticleFavorRow 对应表 user_article_favor_models。
type UserArticleFavorRow struct {
	BaseRow
	ArticleID             ctype.ID           `gorm:"uniqueIndex:uk_user_article_favor,priority:1;index:idx_article_user,priority:1" json:"article_id"`
	UserID                ctype.ID           `gorm:"uniqueIndex:uk_user_article_favor,priority:2;index:idx_article_user,priority:2" json:"user_id"`
	FavorID               ctype.ID           `gorm:"uniqueIndex:uk_user_article_favor,priority:3;index:idx_article_user,priority:3" json:"favor_id"`
	ArticleTitle          string             `gorm:"size:256" json:"article_title"`
	ArticleAbstract       string             `gorm:"size:256" json:"article_abstract"`
	ArticleCover          string             `gorm:"size:256" json:"article_cover"`
	ArticleStatus         enum.ArticleStatus `gorm:"default:0" json:"article_status"`
	ArticleAuthorID       ctype.ID           `gorm:"index" json:"article_author_id"`
	ArticleAuthorNickname string             `gorm:"size:64" json:"article_author_nickname"`
	ArticleAuthorAvatar   string             `gorm:"size:256" json:"article_author_avatar"`
	UserModel             UserRow            `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleModel          ArticleRow         `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
	FavoriteModel         FavoriteRow        `gorm:"foreignKey:FavorID;references:ID" json:"-"`
}

// TableName 固定表名。
func (UserArticleFavorRow) TableName() string { return "user_article_favor_models" }

// UserArticleViewHistoryRow 对应表 user_article_view_history_models。
type UserArticleViewHistoryRow struct {
	BaseRow
	ArticleID    ctype.ID   `gorm:"uniqueIndex:uk_user_article_history,priority:1" json:"article_id"`
	UserID       ctype.ID   `gorm:"uniqueIndex:uk_user_article_history,priority:2" json:"user_id"`
	ArticleModel ArticleRow `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
	UserModel    UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
}

// TableName 固定表名。
func (UserArticleViewHistoryRow) TableName() string { return "user_article_view_history_models" }

// UserFollowRow 对应表 user_follow_models。
type UserFollowRow struct {
	BaseRow
	FollowedUserID    ctype.ID `gorm:"uniqueIndex:uk_user_follow,priority:1" json:"followed_user_id"`
	FollowedNickname  string   `gorm:"size:64" json:"followed_nickname"`
	FollowedAvatar    string   `gorm:"size:256" json:"followed_avatar"`
	FollowedAbstract  string   `gorm:"size:256" json:"followed_abstract"`
	FansUserID        ctype.ID `gorm:"uniqueIndex:uk_user_follow,priority:2" json:"fans_user_id"`
	FansNickname      string   `gorm:"size:64" json:"fans_nickname"`
	FansAvatar        string   `gorm:"size:256" json:"fans_avatar"`
	FansAbstract      string   `gorm:"size:256" json:"fans_abstract"`
	FollowedUserModel UserRow  `gorm:"foreignKey:FollowedUserID;references:ID"`
	FansUserModel     UserRow  `gorm:"foreignKey:FansUserID;references:ID"`
}

// TableName 固定表名。
func (UserFollowRow) TableName() string { return "user_follow_models" }

// UserGlobalNotifRow 对应表 user_global_notif_models。
type UserGlobalNotifRow struct {
	BaseRow
	MsgID  ctype.ID   `gorm:"uniqueIndex:uk_user_global_notif,priority:1" json:"msg_id"`
	UserID ctype.ID   `gorm:"uniqueIndex:uk_user_global_notif,priority:2" json:"user_id"`
	IsRead bool       `json:"is_read"`
	ReadAt *time.Time `json:"read_at"`
}

// TableName 固定表名。
func (UserGlobalNotifRow) TableName() string { return "user_global_notif_models" }

// UserRow 对应表 user_models。
type UserRow struct {
	BaseRow
	Username              string                  `gorm:"size:32;uniqueIndex:uk_user_username" json:"username"`
	Nickname              string                  `gorm:"size:32" json:"nickname"`
	Avatar                string                  `gorm:"size:256" json:"avatar"`
	Abstract              string                  `gorm:"size:256" json:"abstract"`
	RegisterSource        enum.RegisterSourceType `json:"register_source"` // 注册来源
	Password              string                  `gorm:"size:64" json:"-"`
	Email                 *string                 `gorm:"size:256;uniqueIndex:uk_user_email" json:"email"`
	OpenID                *string                 `gorm:"size:64;uniqueIndex:uk_user_open_id" json:"open_id"` // qq 登录的 openid
	Status                enum.UserStatus         `gorm:"default:1;index" json:"status"`
	TokenVersion          uint32                  `gorm:"default:1" json:"token_version"`
	LastPasswordChangedAt *time.Time              `json:"last_password_changed_at"`
	Role                  enum.RoleType           `gorm:"default:0" json:"role"`
	IP                    string                  `gorm:"size:64" json:"ip"`    // 注册时的 IP
	Addr                  string                  `gorm:"size:256" json:"addr"` // 注册时的地址
	UserConfModel         *UserConfRow            `gorm:"foreignKey:UserID;" json:"user_conf_model"`
	UserStatModel         *UserStatRow            `gorm:"foreignKey:UserID;" json:"user_stat_model"`
}

func (u *UserRow) BeforeCreate(tx *gorm.DB) (err error) {
	if u.Status == 0 {
		u.Status = enum.UserStatusActive
	}
	if u.TokenVersion == 0 {
		u.TokenVersion = 1
	}
	return u.BaseRow.BeforeCreate(tx)
}

func (u *UserRow) AfterCreate(tx *gorm.DB) error {
	confModel := UserConfRow{
		UserID:                   u.ID,
		FavoritesVisibility:      true,
		FollowVisibility:         true,
		FansVisibility:           true,
		HomeStyleID:              1,
		DiggNoticeEnabled:        true,
		CommentNoticeEnabled:     true,
		FavorNoticeEnabled:       true,
		PrivateChatNoticeEnabled: true,
		StrangerChatEnabled:      true,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&confModel).Error; err != nil {
		return err
	}

	statModel := UserStatRow{
		UserID:              u.ID,
		ViewCount:           0,
		FansCount:           0,
		FollowCount:         0,
		ArticleVisitedCount: 0,
		ArticleCount:        0,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&statModel).Error
}

// TableName 固定表名。
func (UserRow) TableName() string { return "user_models" }

// UserConfRow 对应表 user_conf_models。
type UserConfRow struct {
	UserID                   ctype.ID   `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	UserModel                UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	LikeTags                 []ctype.ID `gorm:"type:longtext;serializer:json" json:"like_tags"` // 用户偏好标签，关联公共文章标签 ID
	UpdatedUsernameDate      *time.Time `json:"updated_username_date"`                          // 上次修改用户名的时间
	FavoritesVisibility      bool       `json:"favorites_visibility"`                           // 收藏夹是否可见
	FollowVisibility         bool       `json:"followers_visibility"`                           // 关注是否可见
	FansVisibility           bool       `json:"fans_visibility"`                                // 粉丝是否可见
	HomeStyleID              ctype.ID   `json:"home_style_id"`                                  // 首页样式ID
	DiggNoticeEnabled        bool       `json:"digg_notice_enabled"`                            // 是否开启点赞通知
	CommentNoticeEnabled     bool       `json:"comment_notice_enabled"`                         // 是否开启评论通知
	FavorNoticeEnabled       bool       `json:"favor_notice_enabled"`                           // 是否开启收藏通知
	PrivateChatNoticeEnabled bool       `json:"private_chat_notice_enabled"`                    // 是否开启私聊通知
	StrangerChatEnabled      bool       `json:"stranger_msg_enabled"`                           // 是否开启陌生人私聊
}

// TableName 固定表名。
func (UserConfRow) TableName() string { return "user_conf_models" }

// UserSessionRow 对应表 user_session_models。
type UserSessionRow struct {
	BaseRow
	UserID           ctype.ID   `gorm:"index:idx_user_session_user_id;not null" json:"user_id"`
	UserModel        UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	RefreshTokenHash string     `gorm:"size:64; uniqueIndex:uk_user_session_refresh_token" json:"-"` // 服务端记录refresh_token原文，哈希避免明文
	IP               string     `gorm:"size:64" json:"ip"`
	Addr             string     `gorm:"size:256" json:"addr"`
	UA               string     `gorm:"size:512" json:"ua"`
	LastSeenAt       *time.Time `json:"last_seen_at"` // 最近一次活跃时间
	ExpiresAt        time.Time  `gorm:"index:idx_user_session_expires_at" json:"expires_at"`
	RevokedAt        *time.Time `gorm:"index:idx_user_session_revoked_at" json:"revoked_at"` // 被“手动废弃”的时间（注销 / 踢下线）
}

// TableName 固定表名。
func (UserSessionRow) TableName() string { return "user_session_models" }

// UserTopArticleRow 对应表 user_top_article_models。
type UserTopArticleRow struct {
	BaseRow
	UserID       ctype.ID   `gorm:"uniqueIndex:uk_user_top_article,priority:1" json:"user_id"`
	ArticleID    ctype.ID   `gorm:"uniqueIndex:uk_user_top_article,priority:2" json:"article_id"`
	UserModel    UserRow    `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ArticleModel ArticleRow `gorm:"foreignKey:ArticleID;references:ID" json:"-"`
}

// TableName 固定表名。
func (UserTopArticleRow) TableName() string { return "user_top_article_models" }

// DeadLetterRow 是 CdcDeadLetterRow 的别名。
//
// 保留它是因为"死信"在文档与调用方那边一直叫 DeadLetter，
// 而类型名沿用 models 的 CdcDeadLetter* 只是为了对齐真表名。
type DeadLetterRow = CdcDeadLetterRow

// OutboxRow 是 OutboxEventRow 的别名。
//
// 发件箱在业务侧一直叫 outbox，类型名沿用 models 的 OutboxEvent* 只为对齐表名。
type OutboxRow = OutboxEventRow
