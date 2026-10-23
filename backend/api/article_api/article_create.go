package article_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/image_service"
	"myblogx/service/redis_service"
	"myblogx/utils/jwts"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h ArticleApi) ArticleCreateView(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleCreateRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	writer := newArticleWriteService(h.App.DB, h.App.Logger, h.App.RuntimeSite)
	if h.App.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}

	if claims.Role != enum.RoleAdmin && h.App.RuntimeSite.GetRuntimeSite().SiteInfo.Mode == enum.SiteModeBlog {
		res.FailWithMsg("站点处于个人博客模式，普通用户无法创建文章", c)
		return
	}

	article, tagIDs, err := writer.CreateArticle(claims, cr)
	if err != nil {
		switch {
		case errors.Is(err, errArticleUserNotFound), errors.Is(err, errArticleCategoryNotFound), errors.Is(err, errArticleTagInvalid), errors.Is(err, image_service.ErrImageUnavailable):
			res.FailWithMsg(err.Error(), c)
		default:
			res.FailWithMsg("创建文章失败", c)
		}
		return
	}

	applyTagArticleCountDelta(redis_service.NewDeps(h.App.Redis, h.App.Logger), buildTagArticleCountDelta(nil, tagIDs))
	article_score_service.EnsureArticleScoreIfMissingAsync(h.App.DB, h.App.Logger, h.App.RuntimeSite, article.ID)
	coverImageID, _ := image_service.FindImageIDByURL(h.App.DB, article.Cover)

	res.OkWithData(ArticleCreateResponse{
		ID:               article.ID,
		Title:            article.Title,
		CategoryID:       article.CategoryID,
		TagIDs:           append([]ctype.ID(nil), tagIDs...),
		CoverImageID:     coverImageID,
		CommentsToggle:   article.CommentsToggle,
		Status:           article.PublishStatus,
		PublishStatus:    article.PublishStatus,
		VisibilityStatus: article.EffectiveVisibilityStatus(),
	}, c)

	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "article_create",
		TargetType: "article",
		TargetID:   strconv.FormatUint(uint64(article.ID), 10),
		Success:    true,
		Message:    "创建文章成功",
		RequestBody: map[string]any{
			"title":           cr.Title,
			"abstract":        cr.Abstract,
			"cover_image_id":  cr.CoverImageID,
			"category_id":     cr.CategoryID,
			"publish_status":  article.PublishStatus,
			"comments_toggle": cr.CommentsToggle,
			"tag_ids":         cr.TagIDs,
			"content_length":  len(cr.Content),
			"content_changed": len(cr.Content) > 0,
		},
	})
}
