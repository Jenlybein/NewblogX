package app

import (
	"context"

	"myblogx/internal/article/domain"
)

// 本文件是个人数据概览的读面（票 354）。
//
// 为什么要合成一个端点：概览首屏要同时显示状态数量、四个互动计数、最近文章、
// 阅读 Top 5 与 90 天活跃度。之前前端发六七次请求（每个发布态一次搜索、收藏夹
// 一次、用户资料一次……），任何一个慢都会让首屏一半是空的，且"共 N 篇"这类
// 数字来自不同时刻的快照，彼此对不上。
//
// 三条口径：
//  1. **永远限定当前登录用户**：端口形状里写死 userID，路由层也只从登录态取，
//     不接受任何"看别人"的参数 —— 个人概览不该成为越权读入口；
//  2. 不是管理员专属：每个作者都能看自己的；
//  3. 分析存储走 ActivityPort，article 模块不 import 分析模块的 adapter（R11）。

// StudioStatsDays 是活跃度热力图覆盖的天数（13 周 × 7 天）。
const StudioStatsDays = 91

// StudioStats 是个人数据概览的读模型。
type StudioStats struct {
	// StatusCounts 是各发布态的文章数。
	StatusCounts map[domain.Status]int64
	// SumView / SumDigg / SumComment / SumFavor 是四个互动计数之和。
	SumView    int64
	SumDigg    int64
	SumComment int64
	SumFavor   int64
	// Recent 是最近 5 篇（按创建时间倒序）。
	Recent []*domain.Article
	// Top 是阅读量 Top 5。
	Top []*domain.Article
	// Activity 是最近 StudioStatsDays 天里每天的创作动作数，key 是 YYYY-MM-DD。
	//
	// 没有动作的日期**不出现**（由前端补零）：后端返回稀疏数据，前端负责铺满
	// 网格 —— 补零是纯展示逻辑，放在后端只会让载荷白白大一圈。
	Activity map[string]int
}

// StudioStats 读出当前用户的个人数据概览。
//
// 未登录返回 ErrForbidden（不是"返回空数据"：空数据会被前端画成"你还没有内容"，
// 而真实情况是"你没登录"，两者要能分辨）。
func (s *Service) StudioStats(ctx context.Context, viewer domain.Viewer) (*StudioStats, error) {
	if !viewer.IsLoggedIn() {
		return nil, domain.ErrForbidden
	}
	authorID := viewer.UserID

	out := &StudioStats{StatusCounts: map[domain.Status]int64{}, Activity: map[string]int{}}
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos Repos) error {
		stats, err := repos.Articles().AuthorContentStats(ctx, authorID)
		if err != nil {
			return err
		}
		out.StatusCounts = stats.StatusCounts
		out.SumView = stats.SumView
		out.SumDigg = stats.SumDigg
		out.SumComment = stats.SumComment
		out.SumFavor = stats.SumFavor

		recent, err := repos.Articles().ListRecentByAuthor(ctx, authorID, 5)
		if err != nil {
			return err
		}
		out.Recent = recent

		top, err := repos.Articles().ListTopByAuthor(ctx, authorID, 5)
		if err != nil {
			return err
		}
		out.Top = top
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 活跃度单独读、且**失败不致命**：分析库不可用时热力图退回全零，
	// 其余数字照常显示。让整页因为一张热力图打不开是最差的选择。
	if s.activity != nil {
		if days, err := s.activity.DailyActivity(ctx, authorID, StudioStatsDays); err == nil {
			out.Activity = days
		}
	}
	return out, nil
}
