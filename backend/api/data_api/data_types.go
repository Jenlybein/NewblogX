package data_api

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
	Type int8 `form:"type" binding:"required,oneof=1 2 3"`
}

type DateCountItem struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type GrowthDataResponse struct {
	GrowthRate    int             `json:"growth_rate"`
	GrowthNum     int             `json:"growth_num"`
	DateCountList []DateCountItem `json:"date_count_list"`
}

type ArticleYearDataResponse struct {
	DateCountList []DateCountItem `json:"date_count_list"`
}
