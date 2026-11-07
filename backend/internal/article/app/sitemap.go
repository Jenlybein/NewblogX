package app

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
)

// SitemapEntry 是站点地图里的一条 URL（#314 交付 1）。
//
// 只带路径不带域名：域名属于部署形态（本地 / 测试 / 生产各不相同），由
// adapter 从请求的 Host 头现取。用例层拼绝对地址就意味着它得认识"站点域名"
// 这个配置——现在后端没有这个配置，也不该为了一个文件新增一份。
type SitemapEntry struct {
	// Path 是站内路径，例如 `/article/123`。
	Path string
	// Lastmod 是最后修改时间，输出为 <lastmod>。
	Lastmod time.Time
}

// sitemapPageSize 是翻页步长。
//
// 取仓储 Page.Normalize 允许的上限（100）：步长再大也会被收敛回 100，
// 写更大的数字只会让"我设了 500，应该一次查完"变成错觉。
const sitemapPageSize = 100

// Sitemap 列出站点地图应收录的文章路径。
//
// 过滤条件**不是新规则**，而是与公开读面完全相同的两段：
//  1. 仓储 `Filter.OnlyPublic` —— 已发布 + visible（草稿、审核中、被隐藏的都不出）；
//  2. `domain.CanBeSeenByReader` —— 「显示非管理员发布文章」关闭时只出管理员作者，
//     与详情页返回 404 用的是同一个纯函数。
//
// 刻意**不包含**首页以外的其它页面：
//   - 合集 / 项目 / 个人主页 / 搜索页都是 `ssr: false` 的客户端页面（渲染边界，
//     ADR-0019 决策 1）。爬虫打开它们只拿到空壳，列进站点地图只会让站点后台
//     持续报「已提交，未收录」。要收录它们得先扩 SSR 白名单，那是另一张票。
//   - 首页由 adapter 补上（用例层不认识"站点首页"这个概念）。
func (s *Service) Sitemap(ctx context.Context, onlyAdminAuthors bool) ([]SitemapEntry, error) {
	filter := domain.Filter{OnlyPublic: true}
	entries := make([]SitemapEntry, 0, sitemapPageSize)

	err := s.tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		for offset := 0; ; {
			articles, total, err := r.Articles().List(ctx, filter, domain.Page{Offset: offset, Limit: sitemapPageSize})
			if err != nil {
				return err
			}
			if len(articles) == 0 {
				return nil
			}

			// 批量取作者，避免"每篇文章一次"的 N+1：作者是否管理员决定开关过滤。
			authorIDs := make([]domain.UserID, 0, len(articles))
			for _, a := range articles {
				authorIDs = append(authorIDs, a.AuthorID())
			}
			profiles, err := r.Users().Profiles(ctx, authorIDs)
			if err != nil {
				return err
			}

			for _, a := range articles {
				if !domain.CanBeSeenByReader(onlyAdminAuthors, profiles[a.AuthorID()].Admin, false) {
					continue
				}
				entries = append(entries, SitemapEntry{
					Path:    "/article/" + a.ID().String(),
					Lastmod: a.UpdatedAt(),
				})
			}

			offset += len(articles)
			if int64(offset) >= total {
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
