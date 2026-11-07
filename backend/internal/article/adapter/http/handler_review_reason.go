package http

import (
	"myblogx/internal/article/domain/review"
	"myblogx/internal/kernel"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
)

// ReviewReasons 返回驳回原因的固定列表（值 + 文案）。
//
// 为什么要一个端点，而不是让前端自己写一份下拉选项：词表只允许在代码里声明
// 一处（kernel/enum/article_review_enum），前端手抄一份就会在"契约允许的值 /
// 后端认的值 / 下拉里能选的值"之间出现第三种答案。这里把值与文案一起发出去，
// 前端选 value、显示 label，两边都不需要按中文匹配。
//
// 列表是固定枚举，不是运营配置：不允许管理员自定义增删（#302 决策 30）。
//
// @Summary 驳回原因可选值
// @Tags article
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[kernel.ListData[ArticleReviewReasonItem]]
// @Router /article-review/reasons [get]
func (h *Handler) ReviewReasons(c *gin.Context) {
	items := reviewReasonItems()
	res.OkWithData(kernel.ListData[ArticleReviewReasonItem]{List: items, Count: len(items)}, c)
}

// reviewReasonItems 把固定枚举摊成出站结构。
//
// 单独抽出来是为了能单测：端点本身要起路由与鉴权，而"契约里到底给了哪些值"
// 才是这条契约的要害。
func reviewReasonItems() []ArticleReviewReasonItem {
	codes := review.ReasonCodes()
	items := make([]ArticleReviewReasonItem, 0, len(codes))
	for _, code := range codes {
		items = append(items, ArticleReviewReasonItem{Value: code, Label: review.ReasonLabel(code)})
	}
	return items
}
