package global_notif_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func (h GlobalNotifApi) GlobalNotifListView(c *gin.Context) {
	cr := middleware.GetBindQuery[GlobalNotifListRequest](c)

	claims := jwts.MustGetClaimsByGin(c)

	var (
		whereQuery   = h.App.DB.Where("")
		userNotifMap = map[ctype.ID]models.UserGlobalNotifModel{}
	)

	switch cr.Type {
	case 1: // 普通用户能看，且未被删除的通知
		state, err := LoadUserGlobalNotifState(h.App.DB, claims.UserID, nil)
		if err != nil {
			res.FailWithMsg("用户不存在", c)
			return
		}
		userNotifMap = state.UserNotifMap
		whereQuery = BuildUserVisibleGlobalNotifListQuery(h.App.DB, state)
	case 2:
		if !claims.IsAdmin() {
			res.FailWithMsg("权限不足", c)
			return
		}
	}

	_list, hasMore, err := common.ListQueryHasMore(models.GlobalNotifModel{}, common.Options{
		DB:           h.App.DB,
		PageInfo:     cr.PageInfo,
		Likes:        []string{"title", "content"},
		Where:        whereQuery,
		DefaultOrder: "created_at desc",
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	list := make([]GlobalNotifListResponse, 0, len(_list))
	for _, model := range _list {
		userNotif, ok := userNotifMap[model.ID]
		list = append(list, GlobalNotifListResponse{
			ID:       model.ID,
			CreateAt: model.CreatedAt,
			Title:    model.Title,
			Icon:     model.Icon,
			Content:  model.Content,
			Href:     model.Href,
			IsRead:   ok && userNotif.IsRead,
		})
	}

	res.OkWithHasMoreList(list, hasMore, c)
}
