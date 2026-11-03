package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/project"
	"myblogx/internal/kernel"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
)

// 本文件是「项目」的 HTTP 适配层（#193：读者侧公开读）。
//
// 项目与合集并列、页面形态复用，但**数据与表完全独立**；本票只做两个公开读
// 接口（列表 / 详情），管理端增删改与编排属于 #197，挂在独立的管理员分组下。
//
// 读者侧两个接口对匿名开放，但受「展示」开关与「显示非管理员发布文章」
// 开关共同约束：只出展示中的项目，未展示的项目按不存在处理。

// Projects 项目列表（公开读，分页）。
//
// 只返回**展示中**的项目；未展示的项目留在表里（后台 #197 仍可编辑），这里查不到。
// 篇数只算仍公开的文章，封面为空时回退首篇公开文章的封面（都在读模型里算好）。
//
// @Summary 项目列表
// @Tags project
// @Produce json
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} kernel.Envelope[kernel.ListData[ProjectCardResponse]]
// @Router /projects [get]
func (h *Handler) Projects(c *gin.Context) {
	cr := middleware.GetBindQuery[ProjectListRequest](c)

	cards, total, err := h.svc.ListProjectCards(c.Request.Context(),
		h.onlyAdminAuthors(), domain.Page{Offset: pageOffset(cr.Page, cr.Limit), Limit: cr.Limit})
	if err != nil {
		h.fail(c, err)
		return
	}

	out := make([]ProjectCardResponse, 0, len(cards))
	for _, card := range cards {
		out = append(out, ProjectCardResponse{
			ID:           ctype.ID(card.ID),
			Title:        card.Title,
			Description:  card.Description,
			Cover:        card.Cover,
			ArticleCount: card.ArticleCount,
			CreatedAt:    card.CreatedAt,
		})
	}
	res.OkWithList(out, int(total), c)
}

// ProjectDetail 项目详情（公开读）。
//
// 未展示的项目对读者按**不存在**处理（不返回"这个项目未发布"——那能用来探测
// 还没公布的编排）；管理端读面属于 #197。
//
// @Summary 项目详情
// @Tags project
// @Produce json
// @Param id path string true "项目 ID"
// @Success 200 {object} kernel.Envelope[ProjectDetailResponse]
// @Router /projects/{id} [get]
func (h *Handler) ProjectDetail(c *gin.Context) {
	cr := middleware.GetBindUri[kernel.IDRequest](c)

	detail, err := h.svc.ProjectDetail(c.Request.Context(), project.ID(cr.ID),
		h.onlyAdminAuthors())
	if err != nil {
		h.fail(c, err)
		return
	}

	out := ProjectDetailResponse{
		ID:          ctype.ID(detail.Project.ID()),
		Title:       detail.Project.Title(),
		Description: detail.Project.Description(),
		Cover:       detail.Project.Cover(),
		CreatedAt:   detail.Project.CreatedAt(),
		Articles:    make([]ProjectArticleItemResponse, 0, len(detail.Articles)),
	}
	for _, item := range detail.Articles {
		out.Articles = append(out.Articles, ProjectArticleItemResponse{
			ArticleID:      ctype.ID(item.ArticleID),
			SortOrder:      item.SortOrder,
			Title:          item.Title,
			Abstract:       item.Abstract,
			Cover:          item.Cover,
			ViewCount:      item.ViewCount,
			AuthorNickname: item.AuthorNickname,
			CategoryTitle:  item.CategoryTitle,
		})
	}
	res.OkWithData(out, c)
}
