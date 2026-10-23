package models

import (
	"time"

	"myblogx/models/ctype"
)

// UserStatModel 维护用户主页展示所需的冗余统计字段。
// 这些字段是缓存型汇总值，事实来源仍然是关注关系表、日访问去重表等业务表。
type UserStatModel struct {
	UserID              ctype.ID  `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	UserModel           UserModel `gorm:"foreignKey:UserID;references:ID" json:"-"`
	ViewCount           int       `gorm:"default:0" json:"view_count"`
	FansCount           int       `gorm:"default:0" json:"fans_count"`
	FollowCount         int       `gorm:"default:0" json:"follow_count"`
	ArticleVisitedCount int       `gorm:"default:0" json:"article_visited_count"`
	ArticleCount        int       `gorm:"default:0" json:"article_count"`
}

// UserViewDailyModel 记录“某个登录用户在某一天是否访问过某个用户主页”。
// 唯一索引用于数据库兜底，保证同一访客当天对同一主页只会记 1 次。
type UserViewDailyModel struct {
	Model
	UserID       ctype.ID  `gorm:"uniqueIndex:uk_user_view_daily,priority:1" json:"user_id"`
	ViewerUserID ctype.ID  `gorm:"uniqueIndex:uk_user_view_daily,priority:2" json:"viewer_user_id"`
	ViewDate     time.Time `gorm:"type:date;uniqueIndex:uk_user_view_daily,priority:3" json:"view_date"`
}
