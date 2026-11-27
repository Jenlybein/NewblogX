package http

import (
	"context"
	"encoding/json"
	"strings"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/risk"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/imagedisplay"
)

// displayViewer 把文章模块的操作者视图翻译成展示派生器的观看着身份。
//
// 只区分"是不是管理员"：管理员要审图，必须能看到未过审的图；
// 其余人（含作者本人）一律只看已过审的图 —— 规格里明确未过审对作者也不可见。
func displayViewer(v domain.Viewer) imagedisplay.Viewer {
	return imagedisplay.Viewer{IsAdmin: v.IsAdmin()}
}

// displayContent 派生"给读者看的正文"。
//
// 正文里的图片数量不受控（一篇长文可能有几十张），所以先一次性把 src 全取出来
// 批量判定，再统一重写 —— 逐张查库在这里是 N 次往返。
//
// 返回空串表示"没有需要派生的内容"：没有图片时不返回冗余字段，
// 前端回落 content 即可（两者本来一模一样）。
func (h *Handler) displayContent(ctx context.Context, viewer domain.Viewer, html string) string {
	if h.display == nil || strings.TrimSpace(html) == "" {
		return ""
	}
	srcs := imagedisplay.ExtractImgSrcs(html)
	if len(srcs) == 0 {
		return ""
	}
	items := make([]imagedisplay.Item, 0, len(srcs))
	for _, src := range srcs {
		items = append(items, imagedisplay.Item{URL: src, Purpose: imagedisplay.PurposeContent})
	}
	derived := imagedisplay.MustResolve(ctx, h.display, displayViewer(viewer), items)

	resolved := make(map[string]string, len(srcs))
	for i, src := range srcs {
		resolved[src] = derived[i]
	}
	return imagedisplay.RewriteImgSrcs(html, resolved)
}

// toDetailResponse 把用例视图转成详情响应。
//
// CoverImageID / AuthorAvatarImageID 需要把 URL 反查成图片 ID，
// 这是既有响应字段（前端据此判断"这张图是不是本站图库的"），
// 所以不能省。属于集成细节，因此放在 adapter 而不是 app。
func (h *Handler) toDetailResponse(ctx context.Context, viewer domain.Viewer, v *app.ArticleView) ArticleDetailResponse {
	a := v.Article

	resp := ArticleDetailResponse{
		ID:               ctype.NewID(a.ID().Uint64()),
		CreatedAt:        a.CreatedAt(),
		UpdatedAt:        a.UpdatedAt(),
		Title:            a.Title(),
		Abstract:         a.Abstract(),
		Content:          a.Content(),
		CategoryID:       toCtypeIDPtr(a.CategoryID()),
		Cover:            "", // 下面统一派生（样式 + 审核闸门）
		ViewCount:        a.Counters().View,
		DiggCount:        a.Counters().Digg,
		CommentCount:     a.Counters().Comment,
		FavorCount:       a.Counters().Favor,
		CommentsToggle:   a.CommentsToggle(),
		Status:           ArticleStatus(a.Status()),
		PublishStatus:    ArticleStatus(a.Status()),
		VisibilityStatus: ArticleVisibility(a.Visibility()),
		AuthorID:         ctype.NewID(v.Author.ID.Uint64()),
		AuthorAvatar:     "", // 上面统一派生
		AuthorAbstract:   v.Author.Abstract,
		AuthorCreatedAt:  v.Author.CreatedAt,
		AuthorNickname:   v.Author.Nickname,
		AuthorUsername:   v.Author.Username,
		CategoryName:     v.CategoryName,
		IsDigg:           v.Digg,
		IsFavor:          v.Favor,
	}

	for _, t := range v.Tags {
		resp.TagIDs = append(resp.TagIDs, ctype.NewID(t.ID.Uint64()))
		resp.Tags = append(resp.Tags, t.Title)
	}

	// 封面与作者头像走同一个派生器（ADR-0034）：一次批量判定"这两张图能不能看"，
	// 再按用途套样式。合成一次调用而不是两次，少一次查库。
	derived := imagedisplay.MustResolve(ctx, h.display, displayViewer(viewer), []imagedisplay.Item{
		{URL: a.Cover(), Purpose: imagedisplay.PurposeCover},
		{URL: v.Author.Avatar, Purpose: imagedisplay.PurposeAvatar},
	})
	resp.Cover = derived[0]
	resp.AuthorAvatar = derived[1]
	resp.ContentDisplay = h.displayContent(ctx, viewer, a.Content())

	// 图片反查失败不影响主体：旧实现也是忽略错误。
	if h.images != nil {
		if a.Cover() != "" {
			if id, err := h.images.FindImageIDByURL(ctx, a.Cover()); err == nil {
				resp.CoverImageID = id
			}
		}
		if v.Author.Avatar != "" {
			if id, err := h.images.FindImageIDByURL(ctx, v.Author.Avatar); err == nil {
				resp.AuthorAvatarImageID = id
			}
		}
	}

	return resp
}

// toCreateResponse 创建结果。
//
// filteredCount 是这次写入被内容过滤替换的处数（票 397）：只给数量、不给命中词。
func toCreateResponse(a app.ArticleView, coverImageID *ctype.ID, filteredCount int) ArticleCreateResponse {
	art := a.Article
	resp := ArticleCreateResponse{
		ID:               ctype.NewID(art.ID().Uint64()),
		Title:            art.Title(),
		CategoryID:       toCtypeIDPtr(art.CategoryID()),
		CoverImageID:     coverImageID,
		CommentsToggle:   art.CommentsToggle(),
		Status:           ArticleStatus(art.Status()),
		PublishStatus:    ArticleStatus(art.Status()),
		VisibilityStatus: ArticleVisibility(art.Visibility()),
		FilteredCount:    filteredCount,
	}
	for _, t := range a.Tags {
		resp.TagIDs = append(resp.TagIDs, ctype.NewID(t.ID.Uint64()))
	}
	return resp
}

// toTaskItem 审核任务项。
//
// 驳回原因拆成"固定枚举值 + 自由文本"两个字段出站：前端按 value 渲染文案，
// 不按中文字符串匹配（改一个字就断）。历史行没有枚举时 reason_code 为 null，
// reason 仍是整段原文，前端照原样显示。
func toTaskItem(t app.ReviewTaskView) ArticleReviewTaskItem {
	task := t.Task
	item := ArticleReviewTaskItem{
		ID:            ctype.NewID(task.ID()),
		ArticleID:     ctype.NewID(task.ArticleID().Uint64()),
		AuthorID:      ctype.NewID(task.AuthorID().Uint64()),
		ArticleTitle:  task.ArticleTitle(),
		AuthorName:    task.AuthorName(),
		PublishStatus: ArticleStatus(task.PublishStatus()),
		Stage:         ReviewTaskStage(task.Stage()),
		Source:        ReviewTaskSource(task.Source()),
		Status:        ReviewTaskStatus(task.Status()),
		Reason:        task.ReasonNote(),
		CreatedAt:     task.CreatedAt(),
		ReviewedAt:    task.ReviewedAt(),
	}
	if code := task.ReasonCode(); code != "" {
		item.ReasonCode = &code
	}
	if by := task.ReviewedBy(); by != nil {
		id := ctype.NewID(by.Uint64())
		item.ReviewedBy = &id
	}
	// 放行留痕单独出站：不要混进 reason，否则前端会把"越级放行"当成"驳回原因"。
	item.ApproveNote = task.ApproveNote()
	// 分数与放行线是列表视图里的两个事实：nil 分数照原样写 null（omitempty
	// 让它整个字段消失），前端据此区分"还没有分数"与"0 分"。
	item.QualityScore = t.QualityScore
	item.QualityPassScore = t.QualityPassScore
	item.RiskLevel = ReviewRiskLevel(t.Risk.Level)
	// nil → []：契约里这是数组，空列表比 null 更好消费（前端不用再判空）。
	labels := t.Risk.Labels()
	if labels == nil {
		labels = []string{}
	}
	item.RiskLabels = labels
	item.Compliance = toReviewCompliance(t.Compliance)
	return item
}

// toReviewCompliance 把合规记录转成展示形状；没有记录时返回 nil（未判定）。
//
// categories / reasons 存的是 JSON 数组文本：解析失败时按空数组处理而不是
// 报错——一条历史脏数据不该让整个审核列表 500。
func toReviewCompliance(record *risk.Record) *ArticleReviewCompliance {
	if record == nil {
		return nil
	}
	out := &ArticleReviewCompliance{
		Verdict:       string(record.Verdict),
		RiskLevel:     record.RiskLevel,
		Categories:    decodeStringArray(record.CategoriesJSON),
		Reasons:       decodeStringArray(record.ReasonsJSON),
		PromptVersion: record.PromptVersion,
		ModelName:     record.ModelName,
	}
	if !record.CheckedAt.IsZero() {
		checkedAt := record.CheckedAt
		out.CheckedAt = &checkedAt
	}
	return out
}

// decodeStringArray 解析 JSON 字符串数组；失败或空串一律返回空数组。
func decodeStringArray(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	if items == nil {
		return []string{}
	}
	return items
}

// toCtypeIDPtr 转换可空分类 ID。
func toCtypeIDPtr(id *domain.CategoryID) *ctype.ID {
	if id == nil {
		return nil
	}
	v := ctype.NewID(id.Uint64())
	return &v
}
