package app

import (
	"context"
	"strings"

	"myblogx/internal/user/domain"
)

// PublicUserItem 是用户搜索结果里的一行，只含公开字段。
//
// 刻意不带邮箱、状态、角色、令牌版本：搜索是匿名可用的读面，一旦把 User 原样
// 递出去，前端顺手渲染一个字段就是信息泄露。
type PublicUserItem struct {
	ID       domain.UserID
	Nickname string
	Avatar   string
	Abstract string
	CodeAge  int
}

// SearchUsers 按昵称 / 用户名搜索**可公开访问**的用户（只查正常状态）。
//
// 只给公开用户列表用：封禁与注销的账号不出现在搜索结果里，也不返回任何私密字段。
// 关键词为空时直接返回空页 —— 不做"空关键词列出全站用户"这种事。
func (s *Service) SearchUsers(ctx context.Context, keyword string, page, size int) ([]PublicUserItem, int64, error) {
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return []PublicUserItem{}, 0, nil
	}

	active := domain.StatusActive
	result, err := s.users.List(ctx, domain.Query{
		Keyword: kw,
		Status:  &active,
		Page:    page,
		Size:    size,
	})
	if err != nil {
		return nil, 0, err
	}

	now := s.clock.Now()
	items := make([]PublicUserItem, 0, len(result.Items))
	for _, u := range result.Items {
		items = append(items, PublicUserItem{
			ID:       u.ID,
			Nickname: u.Nickname,
			Avatar:   u.Avatar,
			Abstract: u.Abstract,
			CodeAge:  u.CodeAge(now),
		})
	}
	return items, result.Total, nil
}
