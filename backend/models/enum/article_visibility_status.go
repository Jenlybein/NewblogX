package enum

type ArticleVisibilityStatus string

const (
	ArticleVisibilityVisible     ArticleVisibilityStatus = "visible"
	ArticleVisibilityUserHidden  ArticleVisibilityStatus = "user_hidden"
	ArticleVisibilityAdminHidden ArticleVisibilityStatus = "admin_hidden"
)
