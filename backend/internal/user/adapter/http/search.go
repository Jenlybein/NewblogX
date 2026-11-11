package http

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// UserSearchItemResponse 是用户搜索结果的一行。只含公开字段。
type UserSearchItemResponse struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Abstract string `json:"abstract"`
	CodeAge  int    `json:"code_age"`
}

// SearchUsers 按昵称 / 用户名搜索用户（公开读）。
//
// 顶栏搜索的「用户」分栏用它：匿名也能用，只返回正常状态的账号，
// 不返回邮箱、状态、角色等私密字段。关键词为空时返回空列表。
// @Summary 搜索用户
// @Tags user
// @Produce json
// @Param key query string false "关键词（昵称或用户名）"
// @Param page query int false "页码"
// @Param limit query int false "每页数量"
// @Success 200 {object} kernel.Envelope[kernel.HasMoreListData[UserSearchItemResponse]]
// @Router /users/search [get]
func (h *Handler) SearchUsers(c *gin.Context) {
	key := strings.TrimSpace(c.Query("key"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if limit < 1 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}

	items, total, err := h.svc.SearchUsers(c.Request.Context(), key, page, limit)
	if err != nil {
		fail(c, err)
		return
	}

	list := make([]UserSearchItemResponse, 0, len(items))
	for _, item := range items {
		list = append(list, UserSearchItemResponse{
			ID:       strconv.FormatUint(item.ID.Uint64(), 10),
			Nickname: item.Nickname,
			Avatar:   item.Avatar,
			Abstract: item.Abstract,
			CodeAge:  item.CodeAge,
		})
	}
	res.OkWithHasMoreList(list, int64(page*limit) < total, c)
}
