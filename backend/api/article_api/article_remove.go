package article_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/internal/platform/redis"
	"myblogx/internal/platform/redis/redis_article"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/article_service"
	"myblogx/service/user_service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ArticleRemoveView 管理员批量删除文章。
// @Summary 批量删除文章
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body models.IDListRequest true "ID 列表"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /articles [delete]
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
		ActionName: "article_admin_remove",
		TargetType: "article",
		Success:    true,
		Message:    fmt.Sprintf("文章删除成功, 成功删除%d条", len(list)),
	})
}
