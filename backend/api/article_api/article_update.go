package article_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/image_service"
	"myblogx/service/read_service"
	"myblogx/service/redis_service"
	"myblogx/utils/jwts"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h ArticleApi) ArticleUpdateView(c *gin.Context) {
	id := middleware.GetBindUri[models.IDRequest](c)
	cr := middleware.GetBindJson[ArticleUpdateRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	writer := newArticleWriteService(h.App.DB, h.App.Logger, h.App.RuntimeSite)
	article, result, err := writer.UpdateArticle(id.ID, claims, cr)
	if err != nil {
		switch {
		case errors.Is(err, errArticleUserNotFound), errors.Is(err, errArticleNotFound), errors.Is(err, errArticleCategoryNotFound), errors.Is(err, errArticleTagInvalid), errors.Is(err, image_service.ErrImageUnavailable):
			res.FailWithMsg(err.Error(), c)
		default:
			res.FailWithMsg("更新文章失败", c)
		}
		return
	}

	if result.Noop {
		res.OkWithMsg("更新文章成功", c)
		return
	}

	if result.TagChanged {
		applyTagArticleCountDelta(redis_service.NewDeps(h.App.Redis, h.App.Logger), buildTagArticleCountDelta(result.OldTagIDs, result.NewTagIDs))
	}
	if err := read_service.SyncArticleFavorSnapshots(h.App.DB, []ctype.ID{article.ID}); err != nil {
		h.App.Logger.Errorf("同步文章收藏快照失败: 文章ID=%d 错误=%v", article.ID, err)
	}
	if publishStatus, ok := result.UpdateMap["publish_status"].(enum.ArticleStatus); ok && publishStatus == enum.ArticleStatusPublished {
		article_score_service.EnsureArticleScoreIfMissingAsync(h.App.DB, h.App.Logger, h.App.RuntimeSite, article.ID)
	}
	res.OkWithMsg("更新文章成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "article_update",
		TargetType: "article",
		TargetID:   strconv.FormatUint(uint64(article.ID), 10),
		Success:    true,
		Message:    "更新文章成功",
		RequestBody: map[string]any{
			"title":           cr.Title,
			"abstract":        cr.Abstract,
			"cover_image_id":  cr.CoverImageID,
			"category_id":     cr.CategoryID,
			"publish_status":  result.UpdateMap["publish_status"],
			"comments_toggle": cr.CommentsToggle,
			"tag_ids":         cr.TagIDs,
			"content_length": func() int {
				if cr.Content == nil {
					return 0
				}
				return len(*cr.Content)
			}(),
			"content_changed": cr.Content != nil,
		},
	})
}
