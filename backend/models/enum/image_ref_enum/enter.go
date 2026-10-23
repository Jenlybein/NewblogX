package image_ref_enum

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
