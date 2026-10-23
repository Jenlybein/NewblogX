package article_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/image_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h ArticleApi) ArticleDetailView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)

	var article models.ArticleModel
	if err := h.App.DB.Select(
		"ID",
		"CreatedAt",
		"UpdatedAt",
		"Title",
		"Abstract",
		"Content",
		"CategoryID",
		"Cover",
		"AuthorID",
		"ViewCount",
		"DiggCount",
		"CommentCount",
		"FavorCount",
		"CommentsToggle",
		"PublishStatus",
		"VisibilityStatus",
	).Preload("UserModel").
		Preload("CategoryModel").
		Preload("Tags", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort desc, id asc")
		}).
		Take(&article, cr.ID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	var claims *jwts.MyClaims
	token := jwts.GetTokenByGin(c)
	if token != "" {
		authenticator := user_service.NewAuthenticator(
			h.App.DB,
			h.App.Logger,
			h.App.JWT,
			redis_service.Deps{Client: h.App.Redis, Logger: h.App.Logger},
		)
		if authResult, err := authenticator.AuthenticateAccessToken(token); err == nil {
			claims = authResult.Claims
		}
	}
	if claims == nil {
		if !article.IsPublicVisible() {
			res.FailWithMsg("文章不存在", c)
			return
		}
	} else if claims.Role == enum.RoleUser && article.AuthorID != claims.UserID && !article.IsPublicVisible() {
		res.FailWithMsg("文章不存在", c)
		return
	}

	// 获取 redis 里的点赞、收藏、评论数增量
	counters := redis_article.GetBatchCounters(redis_service.NewDeps(h.App.Redis, h.App.Logger), []ctype.ID{article.ID})
	article.DiggCount += counters.DiggMap[article.ID]
	article.ViewCount += counters.ViewMap[article.ID]
	article.FavorCount += counters.FavorMap[article.ID]
	article.CommentCount += counters.CommentMap[article.ID]

	// 是否点赞, 是否收藏
	isDigg := false
	isFavor := false
	if claims != nil {
		var diggCount int64
		if err := h.App.DB.Model(&models.ArticleDiggModel{}).
			Where("article_id = ? AND user_id = ?", article.ID, claims.UserID).
			Count(&diggCount).Error; err == nil {
			isDigg = diggCount > 0
		}

		var favorCount int64
		if err := h.App.DB.Model(&models.UserArticleFavorModel{}).
			Where("article_id = ? AND user_id = ?", article.ID, claims.UserID).
			Count(&favorCount).Error; err == nil {
			isFavor = favorCount > 0
		}
	}

	response := ArticleDetailResponse{
		ID:               article.ID,
		CreatedAt:        article.CreatedAt,
		UpdatedAt:        article.UpdatedAt,
		Title:            article.Title,
		Abstract:         article.Abstract,
		Content:          article.Content,
		CategoryID:       article.CategoryID,
		Cover:            article.Cover,
		ViewCount:        article.ViewCount,
		DiggCount:        article.DiggCount,
		CommentCount:     article.CommentCount,
		FavorCount:       article.FavorCount,
		CommentsToggle:   article.CommentsToggle,
		Status:           article.PublishStatus,
		PublishStatus:    article.PublishStatus,
		VisibilityStatus: article.EffectiveVisibilityStatus(),
		AuthorID:         article.AuthorID,
		AuthorAvatar:     article.UserModel.Avatar,
		AuthorAbstract:   article.UserModel.Abstract,
		AuthorCreatedAt:  article.UserModel.CreatedAt,
		AuthorNickname:   article.UserModel.Nickname,
		AuthorUsername:   article.UserModel.Username,
		IsDigg:           isDigg,
		IsFavor:          isFavor,
	}
	response.CoverImageID, _ = image_service.FindImageIDByURL(h.App.DB, article.Cover)
	response.AuthorAvatarImageID, _ = image_service.FindImageIDByURL(h.App.DB, article.UserModel.Avatar)
	if article.CategoryModel != nil {
		response.CategoryName = article.CategoryModel.Title
	}
	if article.Tags != nil {
		for _, tag := range article.Tags {
			response.TagIDs = append(response.TagIDs, tag.ID)
			response.Tags = append(response.Tags, tag.Title)
		}
	}

	res.OkWithData(response, c)
}
