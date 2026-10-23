package favorite

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h FavoriteApi) FavoriteRemovePatchView(c *gin.Context) {
	var cr = middleware.GetBindJson[FavoriteRemovePatchModel](c)

	if len(cr.Articles) == 0 {
		res.FailWithMsg("请选择要取消收藏的文章", c)
		return
	}

	claims := jwts.MustGetClaimsByGin(c)

	var favoriteModel models.FavoriteModel
	if err := h.App.DB.Take(&favoriteModel, "id = ?", cr.FavoriteID).Error; err != nil {
		res.FailWithMsg("收藏夹不存在", c)
		return
	}
	if !claims.IsAdmin() && favoriteModel.UserID != claims.UserID {
		res.FailWithMsg("权限不足", c)
		return
	}

	var relationList []models.UserArticleFavorModel
	if err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("favor_id = ? AND article_id IN ?", cr.FavoriteID, cr.Articles)
		if !claims.IsAdmin() {
			query = query.Where("user_id = ?", claims.UserID)
		}

		if err := query.Find(&relationList).Error; err != nil {
			return err
		}
		if len(relationList) == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Delete(&relationList).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.FavoriteModel{}).
			Where("id = ?", cr.FavoriteID).
			UpdateColumn("article_count", gorm.Expr("CASE WHEN article_count >= ? THEN article_count - ? ELSE 0 END", len(relationList), len(relationList))).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			res.FailWithMsg("未找到需要取消收藏的文章", c)
			return
		}
		h.App.Logger.Errorf("批量取消收藏失败: 收藏夹ID=%d 错误=%v", cr.FavoriteID, err)
		res.FailWithMsg("批量取消收藏失败", c)
		return
	}

	for _, relation := range relationList {
		if err := redis_article.SetCacheFavorite(redis_service.NewDeps(h.App.Redis, h.App.Logger), relation.ArticleID, -1); err != nil {
			h.App.Logger.Errorf("更新文章收藏缓存失败: 文章ID=%d 错误=%v", relation.ArticleID, err)
		}
	}

	res.OkWithMsg(fmt.Sprintf("批量取消收藏成功，共移除 %d 篇文章", len(relationList)), c)
}
