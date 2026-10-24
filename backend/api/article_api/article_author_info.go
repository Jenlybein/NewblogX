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

// ArticleAuthorInfoView 查询指定作者的文章与粉丝统计信息。
// @Summary 查询作者信息
// @Tags article
// @Produce json
// @Param author_id query int true "作者 ID"
// @Success 200 {object} object{code=int,data=ArticleAuthorInfoResponse,msg=string}
// @Router /articles/author_info [get]
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
