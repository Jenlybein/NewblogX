package article_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/article_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h ArticleApi) ArticleRemoveUserView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)

	claims := jwts.GetClaimsByGin(c)

	var model models.ArticleModel
	if err := h.App.DB.Take(&model, "author_id = ? and id = ?", claims.UserID, cr.ID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}
	viewDelta := redis_article.GetCacheView(redis_service.NewDeps(h.App.Redis, h.App.Logger), model.ID)

	// 软删除
	if err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		if err := article_service.DeleteArticles(tx, []models.ArticleModel{model}, false); err != nil {
			return err
		}
		return user_service.StatApplyArticleDelta(tx, model.AuthorID, -1, -(model.ViewCount + viewDelta))
	}); err != nil {
		res.FailWithMsg("删除文章失败", c)
		return
	}
	res.OkWithMsg("文章删除成功", c)
}
