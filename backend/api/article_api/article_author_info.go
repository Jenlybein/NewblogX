package article_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

type ArticleAuthorInfoResponse struct {
	AuthorID            string `json:"author_id"`
	ArticleCount        int    `json:"article_count"`
	ArticleVisitedCount int    `json:"article_visited_count"`
	FansCount           int    `json:"fans_count"`
}

func (h ArticleApi) ArticleAuthorInfoView(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleAuthorInfoBindRequest](c)

	var user models.UserModel
	if err := h.App.DB.Preload("UserStatModel").Take(&user, cr.AuthorID).Error; err != nil {
		res.FailWithMsg("作者不存在", c)
		return
	}

	var stat models.UserStatModel
	if user.UserStatModel != nil {
		stat = *user.UserStatModel
	}

	res.OkWithData(ArticleAuthorInfoResponse{
		AuthorID:            user.ID.String(),
		ArticleCount:        stat.ArticleCount,
		ArticleVisitedCount: stat.ArticleVisitedCount,
		FansCount:           stat.FansCount,
	}, c)
}
