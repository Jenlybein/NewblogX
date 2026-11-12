package http

import "github.com/gin-gonic/gin"

// SumRequest 是站点汇总的查询参数。
type SumRequest struct {
	// Range 只影响"新增"类数字的起点（票 349）：累计数本身与窗口无关。
	Range string `form:"range" enums:"today,week,month,year"`
}

type SumResponse struct {
	// Range 是收敛后的时间窗，前端据此高亮按钮。
	Range         string `json:"range"`
	FlowCount     int    `json:"flow_count"`
	UserCount     int64  `json:"user_count"`
	ArticleCount  int64  `json:"article_count"`
	MessageCount  int64  `json:"message_count"`
	CommentCount  int64  `json:"comment_count"`
	NewLoginCount int64  `json:"new_login_count"`
	NewSignCount  int64  `json:"new_sign_count"`
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

// bindQuery 读查询参数，**中间件没挂时返回零值而不是 panic**。
//
// middleware.GetBindQuery 是 MustGet：路由漏挂 BindQuery 时它直接 panic，
// gin 的 Recovery 把它变成 500。2026-11-12 线上 `/api/data/sum` 就是这么挂的 ——
// 一个漏挂的中间件让整张汇总卡变成「汇总读取失败」+ 一排 0。
//
// 这里退一步：拿不到绑定值就按默认值跑（range 回落 week、type 缺失由 switch 兜底），
// 页面最多显示得保守一些，而不是整块数据消失。路由该挂的中间件仍然要挂。
func bindQuery[T any](c *gin.Context) T {
	var zero T
	v, ok := c.Get("requestQuery")
	if !ok {
		return zero
	}
	typed, ok := v.(T)
	if !ok {
		return zero
	}
	return typed
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
