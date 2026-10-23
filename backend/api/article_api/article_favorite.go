package article_api

import (
	"errors"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	dbservice "myblogx/service/db_service"
	"myblogx/service/message_service"
	"myblogx/service/read_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h ArticleApi) ArticleFavoriteSaveView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[ArticleFavoriteRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	var article models.ArticleModel
	if err := app.DB.Select("id", "author_id", "title", "abstract", "cover", "publish_status", "visibility_status").
		Take(&article, "id = ?", cr.ArticleID).Error; err != nil {
		res.FailWithMsg("查询文章失败", c)
		return
	}
	if !article.IsPublicVisible() {
		res.FailWithMsg("查询文章失败", c)
		return
	}
	userMap, err := read_service.LoadUserDisplayMap(app.DB, []ctype.ID{claims.UserID, article.AuthorID})
	if err != nil {
		res.FailWithMsg("查询用户信息失败", c)
		return
	}
	actionUser := userMap[claims.UserID]
	authorUser := userMap[article.AuthorID]

	var isFavorited bool
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		favorite, err := getOrCreateFavoriteWithOwner(tx, cr.FavorID, claims.UserID, actionUser)
		if err != nil {
			return err
		}

		isFavorited, err = switchArticleFavorite(tx, article, claims.UserID, favorite.ID, authorUser)
		return err
	}); err != nil {
		res.FailWithMsg("收藏操作失败", c)
		return
	}

	if isFavorited {
		go message_service.InsertArticleFavorMessage(app.DB, app.Logger, message_service.ArticleFavorMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: claims.UserID,
			ArticleID:    cr.ArticleID,
			ArticleTitle: article.Title,
		})

		if err := redis_article.SetCacheFavorite(redis_service.NewDeps(h.App.Redis, h.App.Logger), cr.ArticleID, 1); err != nil {
			app.Logger.Errorf("文章收藏数据加一失败: 错误=%v", err)
		}
		res.OkWithMsg("收藏成功", c)
	} else {
		if err := redis_article.SetCacheFavorite(redis_service.NewDeps(h.App.Redis, h.App.Logger), cr.ArticleID, -1); err != nil {
			app.Logger.Errorf("文章收藏数据减一失败: 错误=%v", err)
		}
		res.OkWithMsg("取消收藏成功", c)
	}
}

// getOrCreateFavoriteID 获取收藏夹；如果获取的是默认收藏夹，不存在就新建
func getOrCreateFavoriteID(db *gorm.DB, favorID, userID ctype.ID) (*models.FavoriteModel, error) {
	userMap, err := read_service.LoadUserDisplayMap(db, []ctype.ID{userID})
	if err != nil {
		return nil, err
	}
	return getOrCreateFavoriteWithOwner(db, favorID, userID, userMap[userID])
}

func getOrCreateFavoriteWithOwner(db *gorm.DB, favorID, userID ctype.ID, owner read_service.UserDisplay) (*models.FavoriteModel, error) {
	var favorite models.FavoriteModel
	if favorID == 0 {
		if err := db.Where("is_default = ? AND user_id = ?", true, userID).
			Attrs(models.FavoriteModel{
				UserID:        userID,
				Title:         "默认收藏夹",
				IsDefault:     true,
				OwnerNickname: owner.Nickname,
				OwnerAvatar:   owner.Avatar,
			}).
			FirstOrCreate(&favorite).Error; err != nil {
			return nil, errors.New("创建默认收藏夹失败")
		}
		if err := db.Model(&favorite).Updates(map[string]any{
			"owner_nickname": owner.Nickname,
			"owner_avatar":   owner.Avatar,
		}).Error; err != nil {
			return nil, errors.New("更新默认收藏夹信息失败")
		}
	} else {
		if err := db.Select("id").Take(&favorite, "id = ? and user_id = ?", favorID, userID).Error; err != nil {
			return nil, errors.New("收藏夹不存在")
		}
	}
	return &favorite, nil
}

// switchArticleFavorite 只处理单个收藏关系的切换，避免收藏夹解析和副作用逻辑混在一起。
func switchArticleFavorite(tx *gorm.DB, article models.ArticleModel, userID, favorID ctype.ID, author read_service.UserDisplay) (bool, error) {
	var articleFavorite models.UserArticleFavorModel
	if err := tx.Select("id").
		Take(&articleFavorite, "article_id = ? and user_id = ? and favor_id = ?", article.ID, userID, favorID).Error; err == nil {
		// 取消收藏必须看本次 Delete 是否真的删掉了活记录，避免并发下双成功。
		deleteResult := tx.Where("article_id = ? and user_id = ? and favor_id = ?", article.ID, userID, favorID).
			Delete(&models.UserArticleFavorModel{})
		if deleteResult.Error != nil {
			return false, deleteResult.Error
		}
		if deleteResult.RowsAffected == 0 {
			return false, errors.New("收藏状态已变化，请刷新后重试")
		}
		if err := tx.Model(&models.FavoriteModel{}).
			Where("id = ?", favorID).
			UpdateColumn("article_count", gorm.Expr("CASE WHEN article_count > 0 THEN article_count - 1 ELSE 0 END")).Error; err != nil {
			return false, err
		}
		return false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}

	// 收藏成功与否只看本次恢复/新建是否真正生效。
	createdOrRestored, err := dbservice.RestoreOrCreateUnique(tx, &models.UserArticleFavorModel{
		ArticleID:             article.ID,
		UserID:                userID,
		FavorID:               favorID,
		ArticleTitle:          article.Title,
		ArticleAbstract:       article.Abstract,
		ArticleCover:          article.Cover,
		ArticleStatus:         article.EffectivePublishStatus(),
		ArticleAuthorID:       article.AuthorID,
		ArticleAuthorNickname: author.Nickname,
		ArticleAuthorAvatar:   author.Avatar,
	}, []string{"article_id", "user_id", "favor_id"})
	if err != nil {
		return false, err
	}
	if !createdOrRestored {
		return false, errors.New("请勿重复收藏")
	}
	if err := tx.Model(&models.UserArticleFavorModel{}).
		Where("article_id = ? and user_id = ? and favor_id = ?", article.ID, userID, favorID).
		Updates(map[string]any{
			"article_title":           article.Title,
			"article_abstract":        article.Abstract,
			"article_cover":           article.Cover,
			"article_status":          article.EffectivePublishStatus(),
			"article_author_id":       article.AuthorID,
			"article_author_nickname": author.Nickname,
			"article_author_avatar":   author.Avatar,
		}).Error; err != nil {
		return false, err
	}
	if err := tx.Model(&models.FavoriteModel{}).
		Where("id = ?", favorID).
		UpdateColumn("article_count", gorm.Expr("article_count + 1")).Error; err != nil {
		return false, err
	}
	return true, nil
}
