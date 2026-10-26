package image_ref_enum

import (
	"fmt"
	"strings"
)

type RefType uint

const (
	RefTypeArticle RefType = iota + 1
	RefTypeUser
	RefTypeBanner
	RefTypeFavorite
)

func (t RefType) String() string {
	switch t {
	case RefTypeArticle:
		return "article"
	case RefTypeUser:
		return "user"
	case RefTypeBanner:
		return "banner"
	case RefTypeFavorite:
		return "favorite"
	default:
		return "unknown"
	}
}

// ParseRefType 把 RefType 的字符串形式解析回枚举。
//
// 死信里存的是 `RefType.String()`（"article" / "user" / ...），重投时要还原成枚举
// 才能查引用表。刻意不接受数字：字符串才是写入时的那份事实，数字是内部表示，
// 两者混着用迟早错位。
func ParseRefType(raw string) (RefType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "article":
		return RefTypeArticle, nil
	case "user":
		return RefTypeUser, nil
	case "banner":
		return RefTypeBanner, nil
	case "favorite":
		return RefTypeFavorite, nil
	default:
		return 0, fmt.Errorf("image_ref_enum: unknown ref type %q", raw)
	}
}

type RefField uint

const (
	RefFieldArticleContent RefField = iota + 1
	RefFieldArticleCover
	RefFieldUserAvatar
	RefFieldBannerCover
	RefFieldFavoriteCover
)

func (f RefField) String() string {
	switch f {
	case RefFieldArticleContent:
		return "article_content"
	case RefFieldArticleCover:
		return "article_cover"
	case RefFieldUserAvatar:
		return "user_avatar"
	case RefFieldBannerCover:
		return "banner_cover"
	case RefFieldFavoriteCover:
		return "favorite_cover"
	default:
		return "unknown"
	}
}
