package user_man_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/log_service"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *UserManApi) UserListView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindQuery[UserListRequest](c)

	_list, count, err := common.ListQuery(models.UserModel{}, common.Options{
		DB:       app.DB,
		Likes:    []string{"nickname", "username"},
		PageInfo: cr.PageInfo,
	})
	if err != nil {
		app.Logger.Errorf("查询用户列表失败: %v", err)
		res.FailWithMsg("查询用户列表失败", c)
		return
	}

	idList := make([]ctype.ID, 0, len(_list))
	for _, item := range _list {
		idList = append(idList, item.ID)
	}
	latestLoginMap, err := log_service.LoadLatestLoginMap(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), idList)
	if err != nil {
		app.Logger.Errorf("加载用户最后登录信息失败: %v", err)
	}

	var list = make([]UserListResponse, 0)
	for _, item := range _list {
		data := UserListResponse{
			ID:        item.ID,
			Nickname:  item.Nickname,
			Avatar:    item.Avatar,
			Username:  item.Username,
			CreatedAt: item.CreatedAt,
		}
		if lastLogin, ok := latestLoginMap[item.ID]; ok {
			data.IP = lastLogin.IP
			data.Addr = lastLogin.Addr
			if parsedAt, parseErr := time.ParseInLocation("2006-01-02 15:04:05.000", lastLogin.TS, time.Local); parseErr == nil {
				data.LastLoginAt = parsedAt
			}
		}
		list = append(list, data)
	}

	res.OkWithList(list, count, c)
}
