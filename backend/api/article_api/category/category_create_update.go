package category

import (
	"errors"
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 创建或者编辑分类（不传入 ID 视为创建，传入 ID 视为编辑）
func (h CategoryApi) CategoryCreateUpdateView(c *gin.Context) {
	cr := middleware.GetBindJson[CategoryRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	// 创建
	if cr.ID == 0 {
		category := models.CategoryModel{
			Title:  cr.Title,
			UserID: claims.UserID,
		}
		// 分类创建改为直接创建新记录，不再恢复同名软删数据。
		if err := h.App.DB.Create(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				res.FailWithMsg("分类名称重复", c)
				return
			}
			res.FailWithMsg(fmt.Sprintf("创建分类失败 %v", err), c)
			return
		}
		res.OkWithData(CategoryCreateResponse{
			ID:    category.ID,
			Title: category.Title,
		}, c)
		middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
			ActionName:        "category_create",
			TargetType:        "category",
			TargetID:          strconv.FormatUint(uint64(category.ID), 10),
			Success:           true,
			Message:           "创建分类成功",
			RequestBody:       cr,
			UseRawRequestBody: true,
		})
		return
	}

	// 编辑
	var category models.CategoryModel
	if err := h.App.DB.Take(&category, "user_id = ? and id = ?", claims.UserID, cr.ID).Error; err != nil {
		res.FailWithMsg("分类不存在", c)
		return
	}

	if err := h.App.DB.Model(&category).Update("title", cr.Title).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			res.FailWithMsg("分类名称重复", c)
			return
		}
		res.FailWithMsg(fmt.Sprintf("更新分类失败 %v", err), c)
		return
	}
	res.OkWithMsg("更新分类成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName:        "category_update",
		TargetType:        "category",
		TargetID:          strconv.FormatUint(uint64(category.ID), 10),
		Success:           true,
		Message:           "更新分类成功",
		RequestBody:       cr,
		UseRawRequestBody: true,
	})
}
