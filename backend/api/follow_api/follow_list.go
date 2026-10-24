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
// FollowListView 获取关注列表
// @Summary 关注列表
// @Tags follow
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param user_id query int false "目标用户 ID，缺省为当前登录用户"
// @Param followed_user_id query int false "只查与指定被关注用户相关的记录"
// @Success 200 {object} object{code=int,data=object{list=[]follow_service.FollowListItem,has_more=bool},msg=string}
// @Router /follow [get]
func (h *FollowApi) FollowListView(c *gin.Context) {
	cr := middleware.GetBindQuery[FollowListRequest](c)

	claims := jwts.GetClaimsByGin(c)

	// 查询目标用户隐私设置，判断是否公开关注列表
	if cr.UserID != claims.UserID {
		if cr.UserID != 0 {
			var user models.UserConfModel
			if err := h.App.DB.Take(&user, "user_id = ?", cr.UserID).Error; err != nil {
				res.FailWithMsg("用户配置信息不存在", c)
				return
			}
			if !user.FollowVisibility {
				res.FailWithMsg("关注列表不公开", c)
				return
			}
		} else {
			cr.UserID = claims.UserID
		}
	}

	queryService := follow_service.NewQueryService(h.App.DB)
	list, hasMore, err := queryService.ListFollowing(cr.UserID, claims.UserID, cr.FollowedUserID, cr.PageInfo)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithHasMoreList(list, hasMore, c)
}
