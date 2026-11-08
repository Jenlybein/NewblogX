package enum

type ArticleVisibilityStatus string

const (
	ArticleVisibilityVisible     ArticleVisibilityStatus = "visible"
	ArticleVisibilityUserHidden  ArticleVisibilityStatus = "user_hidden"
	ArticleVisibilityAdminHidden ArticleVisibilityStatus = "admin_hidden"
	// ArticleVisibilityPendingReview 是"已发布，但等模型合规判定通过才对读者可见"。
	//
	// 它不是新状态、也不是新列：复用 visibility_status 的 varchar(32)，
	// 因此不触碰零 DDL 约束。提交时同步扫不到风险的文章先进这个中间态，
	// 模型判 clean 才转 visible；判 suspect / 判定失败则改回审核中交人工。
	// 作者本人和管理员始终可见（CanBeViewedBy 的既有豁免），只有匿名访客
	// 与其他读者被挡住。
	ArticleVisibilityPendingReview ArticleVisibilityStatus = "pending_review"
)
