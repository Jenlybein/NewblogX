package article_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/article_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h ArticleApi) ArticleRemoveView(c *gin.Context) {
	cr := middleware.GetBindJson[models.IDListRequest](c)

	var list []models.ArticleModel
	h.App.DB.Find(&list, "id in ?", cr.IDList)

	if len(list) == 0 {
		res.FailWithMsg("删除失败，文章不存在", c)
		return
	}
	authorDeltas := buildAuthorStatDeltaMap(redis_article.GetBatchCacheView(redis_service.NewDeps(h.App.Redis, h.App.Logger), collectArticleIDs(list)), list)
	if err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		if err := article_service.DeleteArticles(tx, list, false); err != nil {
			return err
		}
		for authorID, delta := range authorDeltas {
			if err := user_service.StatApplyArticleDelta(tx, authorID, -delta.ArticleCount, -delta.ArticleVisitedCount); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		res.FailWithMsg("删除文章失败", c)
		return
	}
	res.OkWithMsg(fmt.Sprintf("文章删除成功, 成功删除%d条", len(list)), c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName:  "article_admin_remove",
		TargetType:  "article",
		Success:     true,
		Message:     fmt.Sprintf("文章删除成功, 成功删除%d条", len(list)),
		RequestBody: map[string]any{"id_list": cr.IDList},
		ResponseBody: map[string]any{
			"deleted_count": len(list),
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}
