package article_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/internal/platform/redis"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/image_service"
	"myblogx/service/read_service"
	"myblogx/utils/jwts"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ArticleUpdateView 更新文章。
// @Summary 更新文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文章 ID"
// @Param body body ArticleUpdateRequest true "请求体"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles/{id} [put]
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
	})
}
