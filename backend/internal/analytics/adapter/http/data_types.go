package http

type SumResponse struct {
	FlowCount     int   `json:"flow_count"`
	UserCount     int64 `json:"user_count"`
	ArticleCount  int64 `json:"article_count"`
	MessageCount  int64 `json:"message_count"`
	CommentCount  int64 `json:"comment_count"`
	NewLoginCount int64 `json:"new_login_count"`
	NewSignCount  int64 `json:"new_sign_count"`
}

type GrowthDataRequest struct {
	// 1 网站流量 2 文章发布 3 用户注册
	Type int8 `form:"type" binding:"required,oneof=1 2 3" enums:"1,2,3"`
	// Range 是时间窗（票 349）：today / week / month / year，空值按 week。
	//
	// 不用 binding 的 oneof 卡死：多一个非法值应当回落到默认窗，而不是让
	// 整张卡片报错 —— 概览页有七张卡，一张卡参数写错不该让其余六张也空着。
	Range string `form:"range" enums:"today,week,month,year"`
}

type DateCountItem struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type GrowthDataResponse struct {
	GrowthRate    int             `json:"growth_rate"`
	GrowthNum     int             `json:"growth_num"`
	DateCountList []DateCountItem `json:"date_count_list"`
	// Range 是**收敛后**的时间窗，前端据此高亮按钮（传了非法值也能知道实际用了哪个）。
	Range string `json:"range"`
}

type ArticleYearDataResponse struct {
	DateCountList []DateCountItem `json:"date_count_list"`
}
