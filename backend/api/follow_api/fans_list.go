package follow_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/service/follow_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// TODO：增加用户名搜索
// FansListView 获取粉丝列表
// @Summary 粉丝列表
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param user_id query int false "目标用户 ID，缺省为当前登录用户"
// @Param fans_user_id query int false "只查与指定粉丝用户相关的记录"
// @Success 200 {object} object{code=int,data=object{list=[]follow_service.FansListItem,has_more=bool},msg=string}
// @Router /fans [get]
func (h *FollowApi) FansListView(c *gin.Context) {
	cr := middleware.GetBindQuery[FansListRequest](c)

	claims := jwts.GetClaimsByGin(c)

	// 查询目标用户隐私设置，判断是否公开粉丝列表
	if cr.UserID != claims.UserID {
		if cr.UserID != 0 {
			var user models.UserConfModel
			if err := h.App.DB.Take(&user, "user_id = ?", cr.UserID).Error; err != nil {
				res.FailWithMsg("用户配置信息不存在", c)
				return
			}
			if !user.FansVisibility {
				res.FailWithMsg("粉丝列表不公开", c)
				return
			}
		} else {
			cr.UserID = claims.UserID
		}
	}

	queryService := follow_service.NewQueryService(h.App.DB)
	list, hasMore, err := queryService.ListFans(cr.UserID, claims.UserID, cr.FansUserID, cr.PageInfo)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}
