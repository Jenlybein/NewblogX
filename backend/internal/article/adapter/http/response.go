package http

import (
	"context"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// toDetailResponse 把用例视图转成详情响应。
//
// CoverImageID / AuthorAvatarImageID 需要把 URL 反查成图片 ID，
// 这是既有响应字段（前端据此判断"这张图是不是本站图库的"），
// 所以不能省。属于集成细节，因此放在 adapter 而不是 app。
func (h *Handler) toDetailResponse(ctx context.Context, v *app.ArticleView) ArticleDetailResponse {
	a := v.Article

	resp := ArticleDetailResponse{
		ID:               ctype.ID(a.ID().Uint64()),
		CreatedAt:        a.CreatedAt(),
		UpdatedAt:        a.UpdatedAt(),
		Title:            a.Title(),
		Abstract:         a.Abstract(),
		Content:          a.Content(),
		CategoryID:       toCtypeIDPtr(a.CategoryID()),
		Cover:            a.Cover(),
		ViewCount:        a.Counters().View,
		DiggCount:        a.Counters().Digg,
		CommentCount:     a.Counters().Comment,
		FavorCount:       a.Counters().Favor,
		CommentsToggle:   a.CommentsToggle(),
		Status:           enum.ArticleStatus(a.Status()),
		PublishStatus:    enum.ArticleStatus(a.Status()),
		VisibilityStatus: enum.ArticleVisibilityStatus(a.Visibility()),
		AuthorID:         ctype.ID(v.Author.ID.Uint64()),
		AuthorAvatar:     v.Author.Avatar,
		AuthorAbstract:   v.Author.Abstract,
		AuthorCreatedAt:  v.Author.CreatedAt,
		AuthorNickname:   v.Author.Nickname,
		AuthorUsername:   v.Author.Username,
		CategoryName:     v.CategoryName,
		IsDigg:           v.Digg,
		IsFavor:          v.Favor,
	}

	for _, t := range v.Tags {
		resp.TagIDs = append(resp.TagIDs, ctype.ID(t.ID.Uint64()))
		resp.Tags = append(resp.Tags, t.Title)
	}

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
func toCreateResponse(a app.ArticleView, coverImageID *ctype.ID) ArticleCreateResponse {
	art := a.Article
	resp := ArticleCreateResponse{
		ID:               ctype.ID(art.ID().Uint64()),
		Title:            art.Title(),
		CategoryID:       toCtypeIDPtr(art.CategoryID()),
		CoverImageID:     coverImageID,
		CommentsToggle:   art.CommentsToggle(),
		Status:           enum.ArticleStatus(art.Status()),
		PublishStatus:    enum.ArticleStatus(art.Status()),
		VisibilityStatus: enum.ArticleVisibilityStatus(art.Visibility()),
	}
	for _, t := range a.Tags {
		resp.TagIDs = append(resp.TagIDs, ctype.ID(t.ID.Uint64()))
	}
	return resp
}

// toTaskItem 审核任务项。
func toTaskItem(t app.ReviewTaskView) ArticleReviewTaskItem {
	task := t.Task
	item := ArticleReviewTaskItem{
		ID:            ctype.ID(task.ID()),
		ArticleID:     ctype.ID(task.ArticleID().Uint64()),
		AuthorID:      ctype.ID(task.AuthorID().Uint64()),
		ArticleTitle:  task.ArticleTitle(),
		AuthorName:    task.AuthorName(),
		PublishStatus: enum.ArticleStatus(task.PublishStatus()),
		Stage:         models.ArticleReviewTaskStage(task.Stage()),
		Source:        models.ArticleReviewTaskSource(task.Source()),
		Status:        models.ArticleReviewTaskStatus(task.Status()),
		Reason:        task.Reason(),
		CreatedAt:     task.CreatedAt(),
		ReviewedAt:    task.ReviewedAt(),
	}
	if by := task.ReviewedBy(); by != nil {
		id := ctype.ID(by.Uint64())
		item.ReviewedBy = &id
	}
	return item
}

// toCtypeIDPtr 转换可空分类 ID。
func toCtypeIDPtr(id *domain.CategoryID) *ctype.ID {
	if id == nil {
		return nil
	}
	v := ctype.ID(id.Uint64())
	return &v
}
