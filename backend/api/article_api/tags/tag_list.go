package tags

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_tag"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h TagsApi) TagListView(c *gin.Context) {
	cr := middleware.GetBindQuery[TagListRequest](c)

	var query *gorm.DB
	if cr.IsEnabled != nil {
		query = h.App.DB.Where("is_enabled = ?", *cr.IsEnabled)
	}

	list, count, err := common.ListQuery(models.TagModel{}, common.Options{
		DB:           h.App.DB,
		Select:       []string{"id", "title", "description", "is_enabled", "sort", "article_count"},
		PageInfo:     cr.PageInfo,
		Likes:        []string{"title"},
		DefaultOrder: "sort desc, id desc",
		Where:        query,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	tagIDs := make([]ctype.ID, 0, len(list))
	for _, item := range list {
		tagIDs = append(tagIDs, item.ID)
	}

	deltaMap := redis_tag.GetBatchCacheArticleCount(redis_service.NewDeps(h.App.Redis, h.App.Logger), tagIDs)
	responseList := make([]TagListResponse, 0, len(list))
	for _, item := range list {
		item.ArticleCount += deltaMap[item.ID]
		if item.ArticleCount < 0 {
			item.ArticleCount = 0
		}
		responseList = append(responseList, TagListResponse{
			TagModel: item,
		})
	}

	res.OkWithList(responseList, count, c)
}
