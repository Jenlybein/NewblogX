package top

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	dbservice "myblogx/service/db_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxUserTopArticleCount = 3

func (h TopApi) ArticleTopSetView(c *gin.Context) {
	cr := middleware.GetBindJson[ArticleTopSetRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	db := h.App.DB
	logger := h.App.Logger

	var article models.ArticleModel
	if err := db.Select("id", "author_id", "publish_status").Take(&article, "id = ?", cr.ArticleID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	switch cr.Type {
	case 1:
		if article.AuthorID != claims.UserID {
			res.FailWithMsg("只能置顶自己的文章", c)
			return
		}
		if article.PublishStatus != enum.ArticleStatusPublished {
			res.FailWithMsg("文章未发布，无法置顶", c)
			return
		}
	case 2:
		if !claims.IsAdmin() {
			res.FailWithMsg("只有管理员才能执行管理员置顶", c)
			return
		}
	default:
		res.FailWithMsg("置顶类型错误", c)
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		// 用户级行锁用于串行化“置顶数量限制 + 当前文章置顶状态”这组组合判断。
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Take(&models.UserModel{}, claims.UserID).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Take(&models.UserTopArticleModel{}, "user_id = ? AND article_id = ?", claims.UserID, article.ID).Error; err == nil {
			return errors.New("文章已被置顶")
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// 检查用户置顶数量
		if cr.Type == 1 && !claims.IsAdmin() {
			if err := tx.Model(&models.UserTopArticleModel{}).
				Where("user_id = ?", claims.UserID).
				Count(&count).Error; err != nil {
				return err
			}
			if count >= maxUserTopArticleCount {
				return errors.New("文章置顶数量达到限制")
			}
		}

		createdOrRestored, err := dbservice.RestoreOrCreateUnique(tx, &models.UserTopArticleModel{
			UserID:    claims.UserID,
			ArticleID: article.ID,
		}, []string{"user_id", "article_id"})
		if err != nil {
			return err
		}
		if !createdOrRestored {
			return errors.New("文章已被置顶")
		}
		return nil
	})
	if err != nil {
		logger.Errorf("文章置顶失败: 文章ID=%d 用户ID=%d 类型=%d 错误=%v", article.ID, claims.UserID, cr.Type, err)
		res.FailWithError(err, c)
		return
	}

	res.OkWithMsg("文章置顶成功", c)
}
