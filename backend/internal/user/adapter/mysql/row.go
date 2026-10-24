package mysql

import (
	"time"

	"myblogx/internal/platform/idgen"

	"gorm.io/gorm"
)

// 本文件是本模块的 row 结构。
//
// 为什么在自己的 adapter 里重新定义、而不是复用 models/：
// models 是所有模块共用的，任何一处改列名都会同时影响别人，最后没人敢动。
// row 归属于"谁的 adapter 读写它"，M6 删 models/ 时这里不用再改一次。
// 代价是字段要抄一遍——所以每一列都标注了它来自哪张表、哪个约束。

// BaseRow 是四张表共用的主键与时间戳。
//
// 必须导出：GORM 通过反射读取匿名字段，非导出类型会被整体忽略，
// 表现为主键列不参与 INSERT，然后 MySQL 报 "Field 'id' doesn't have a default value"。
type BaseRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// beforeCreate 生成雪花 ID：与 models.Model 的钩子行为一致。
func beforeCreate(id *uint64) error {
	if *id != 0 {
		return nil
	}
	next, err := idgen.Next()
	if err != nil {
		return err
	}
	*id = next.Uint64()
	return nil
}

// UserRow 对应 user_models 表。
type UserRow struct {
	BaseRow
	Username              string `gorm:"size:32;uniqueIndex:uk_user_username"`
	Nickname              string `gorm:"size:32"`
	Avatar                string `gorm:"size:256"`
	Abstract              string `gorm:"size:256"`
	RegisterSource        int8
	Password              string  `gorm:"size:64"`
	Email                 *string `gorm:"size:256;uniqueIndex:uk_user_email"`
	OpenID                *string `gorm:"size:64;uniqueIndex:uk_user_open_id"`
	Status                int8    `gorm:"default:1;index"`
	TokenVersion          uint32  `gorm:"default:1"`
	LastPasswordChangedAt *time.Time
	Role                  int8   `gorm:"default:0"`
	IP                    string `gorm:"size:64"`
	Addr                  string `gorm:"size:256"`
}

// TableName 固定表名，避免 GORM 命名策略变化导致查错表。
func (UserRow) TableName() string { return "user_models" }

// BeforeCreate 生成雪花 ID 并补默认值。
func (r *UserRow) BeforeCreate(_ *gorm.DB) error {
	if r.Status == 0 {
		r.Status = 1
	}
	if r.TokenVersion == 0 {
		r.TokenVersion = 1
	}
	return beforeCreate(&r.ID)
}

// UserConfRow 对应 user_conf_models 表。
type UserConfRow struct {
	UserID                   uint64   `gorm:"primaryKey;autoIncrement:false"`
	LikeTags                 []uint64 `gorm:"type:longtext;serializer:json"`
	UpdatedUsernameDate      *time.Time
	FavoritesVisibility      bool
	FollowVisibility         bool
	FansVisibility           bool
	HomeStyleID              uint64
	DiggNoticeEnabled        bool
	CommentNoticeEnabled     bool
	FavorNoticeEnabled       bool
	PrivateChatNoticeEnabled bool
	StrangerChatEnabled      bool
}

// TableName 固定表名。
func (UserConfRow) TableName() string { return "user_conf_models" }

// UserStatRow 对应 user_stat_models 表。
type UserStatRow struct {
	UserID              uint64 `gorm:"primaryKey;autoIncrement:false"`
	ViewCount           int    `gorm:"default:0"`
	FansCount           int    `gorm:"default:0"`
	FollowCount         int    `gorm:"default:0"`
	ArticleVisitedCount int    `gorm:"default:0"`
	ArticleCount        int    `gorm:"default:0"`
}

// TableName 固定表名。
func (UserStatRow) TableName() string { return "user_stat_models" }
