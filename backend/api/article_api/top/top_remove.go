package top

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func (h TopApi) ArticleTopRemoveView(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleTopSetRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	db := h.App.DB
	logger := h.App.Logger

	var article models.ArticleModel
	if err := db.Select("id", "author_id").Take(&article, "id = ?", cr.ArticleID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	switch cr.Type {
	case 1:
		if article.AuthorID != claims.UserID {
			res.FailWithMsg("只能取消自己文章的置顶", c)
			return
		}
	case 2:
		if !claims.IsAdmin() {
			res.FailWithMsg("只有管理员才能取消管理员置顶", c)
			return
		}
	default:
		res.FailWithMsg("置顶类型错误", c)
		return
	}

	result := db.Delete(&models.UserTopArticleModel{}, "user_id = ? AND article_id = ?", claims.UserID, article.ID)
	if result.Error != nil {
		logger.Errorf("取消文章置顶失败: 文章ID=%d 用户ID=%d 类型=%d 错误=%v", article.ID, claims.UserID, cr.Type, result.Error)
		res.FailWithMsg("取消置顶失败", c)
		return
	}
	if result.RowsAffected == 0 {
		res.FailWithMsg("文章未置顶", c)
		return
	}

	res.OkWithMsg("取消置顶成功", c)
}
