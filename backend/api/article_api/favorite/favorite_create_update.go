package favorite

import (
	"errors"
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/read_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 创建或者编辑收藏夹（传入ID则视为创建，不传入则视为编辑）
func (h FavoriteApi) FavoriteCreateUpdateView(c *gin.Context) {
	cr := middleware.GetBindJson[FavoriteRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	// 创建
	if cr.ID == 0 {
		userMap, err := read_service.LoadUserDisplayMap(h.App.DB, []ctype.ID{claims.UserID})
		if err != nil {
			res.FailWithMsg("查询用户信息失败", c)
			return
		}
		user := userMap[claims.UserID]
		// 收藏夹创建改为直接创建新记录，不再恢复同名软删数据。
		favorite := models.FavoriteModel{
			UserID:        claims.UserID,
			Title:         cr.Title,
			Cover:         cr.Cover,
			Abstract:      cr.Abstract,
			OwnerNickname: user.Nickname,
			OwnerAvatar:   user.Avatar,
		}
		if err := h.App.DB.Create(&favorite).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				res.FailWithMsg("收藏夹名称重复", c)
				return
			}
			res.FailWithMsg(fmt.Sprintf("创建收藏夹失败 %v", err), c)
			return
		}
		res.OkWithData(FavoriteCreateResponse{
			ID:           favorite.ID,
			UserID:       favorite.UserID,
			Title:        favorite.Title,
			Cover:        favorite.Cover,
			Abstract:     favorite.Abstract,
			IsDefault:    favorite.IsDefault,
			ArticleCount: favorite.ArticleCount,
		}, c)
		return
	}

	// 编辑
	var favorite models.FavoriteModel
	if err := h.App.DB.Take(&favorite, "user_id = ? and id = ?", claims.UserID, cr.ID).Error; err != nil {
		res.FailWithMsg("收藏夹不存在", c)
		return
	}

	if err := h.App.DB.Model(&favorite).Updates(map[string]any{
		"title":    cr.Title,
		"cover":    cr.Cover,
		"abstract": cr.Abstract,
	}).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			res.FailWithMsg("收藏夹名称重复", c)
			return
		}
		res.FailWithMsg(fmt.Sprintf("更新收藏夹失败 %v", err), c)
		return
	}
	res.OkWithMsg("更新收藏夹成功", c)
}
