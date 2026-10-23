package article_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/message_service"
	"myblogx/service/read_service"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h ArticleApi) ArticleExamineView(c *gin.Context) {
	app := h.App
	id := middleware.GetBindUri[models.IDRequest](c)
	cr := middleware.GetBindJson[ArticleExamineRequest](c)

	var article models.ArticleModel
	if err := app.DB.Take(&article, id.ID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	if err := app.DB.Model(&article).Update("publish_status", cr.Status).Error; err != nil {
		res.FailWithMsg("文章审核失败", c)
		return
	}
	if err := read_service.SyncArticleFavorSnapshots(app.DB, []ctype.ID{article.ID}); err != nil {
		app.Logger.Errorf("同步文章收藏快照失败: 文章ID=%d 错误=%v", article.ID, err)
	}
	if cr.Status == enum.ArticleStatusPublished {
		article_score_service.EnsureArticleScoreIfMissingAsync(app.DB, app.Logger, app.RuntimeSite, article.ID)
	}

	// 给文章创作者发送系统通知
	switch cr.Status {
	case 3: // 审核成功
		go message_service.InsertSystemMessage(app.DB, app.Logger, message_service.SystemMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: &article.AuthorID,
			Content:      fmt.Sprintf("您的文章《%s》审核通过!", article.Title),
			LinkTitle:    article.Title,
			LinkHerf:     fmt.Sprintf("/article/%d", article.ID),
		})
	case 4: // 审核失败
		go message_service.InsertSystemMessage(app.DB, app.Logger, message_service.SystemMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: &article.AuthorID,
			Content:      fmt.Sprintf("您的文章《%s》审核失败，请修改后再提交!\n失败原因：%s", article.Title, ""),
			LinkTitle:    article.Title,
			LinkHerf:     fmt.Sprintf("/article/%d", article.ID),
		})
	}
	res.OkWithMsg("文章审核成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "article_examine",
		TargetType: "article",
		TargetID:   strconv.FormatUint(uint64(article.ID), 10),
		Success:    true,
		Message:    "文章审核成功",
		RequestBody: map[string]any{
			"status": cr.Status,
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}
