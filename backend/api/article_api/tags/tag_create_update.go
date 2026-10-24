package tags

import (
	"errors"
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/utils/jwts"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TagCreateUpdateView 创建或更新标签。
// @Summary 创建/更新标签
// @Tags article
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body tags.TagRequest true "请求体"
// @Success 200 {object} object{code=int,data=tags.TagCreateResponse,msg=string}
// @Router /articles/tags [put]
func (h TagsApi) TagCreateUpdateView(c *gin.Context) {
	cr := middleware.GetBindJson[TagRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	title := strings.TrimSpace(cr.Title)
	if title == "" {
		res.FailWithMsg("标签名称不能为空", c)
		return
	}

	isEnabled := true
	if cr.ID == 0 && cr.IsEnabled != nil {
		isEnabled = *cr.IsEnabled
	}

	if cr.ID == 0 {
		tag := models.TagModel{
			Title:       title,
			Sort:        cr.Sort,
			Description: cr.Description,
			IsEnabled:   isEnabled,
			CreatedBy:   claims.UserID,
		}
		// 标签创建改为直接创建新记录，不再恢复同名软删数据。
		if err := h.App.DB.Create(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				res.FailWithMsg("标签名称重复", c)
				return
			}
			res.FailWithMsg(fmt.Sprintf("创建标签失败: %v", err), c)
			return
		}
		res.OkWithData(TagCreateResponse{
			ID:          tag.ID,
			Title:       tag.Title,
			Sort:        tag.Sort,
			Description: tag.Description,
			IsEnabled:   tag.IsEnabled,
		}, c)
		middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
			ActionName: "tag_create",
			TargetType: "tag",
			TargetID:   strconv.FormatUint(uint64(tag.ID), 10),
			Success:    true,
			Message:    "创建标签成功",
		})
		return
	}

	var tag models.TagModel
	if err := h.App.DB.Take(&tag, cr.ID).Error; err != nil {
		res.FailWithMsg("标签不存在", c)
		return
	}
	if cr.IsEnabled != nil {
		isEnabled = *cr.IsEnabled
	} else {
		isEnabled = tag.IsEnabled
	}

	if err := ensureTagUnique(h.App.DB, tag.ID, title); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	if err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&tag).Updates(map[string]any{
			"title":       title,
			"sort":        cr.Sort,
			"description": cr.Description,
			"is_enabled":  isEnabled,
		}).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return fmt.Errorf("标签名称重复")
			}
			return err
		}
		return nil
	}); err != nil {
		res.FailWithMsg(fmt.Sprintf("更新标签失败: %v", err), c)
		return
	}
	res.OkWithMsg("更新标签成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "tag_update",
		TargetType: "tag",
		TargetID:   strconv.FormatUint(uint64(tag.ID), 10),
		Success:    true,
		Message:    "更新标签成功",
	})
}

func ensureTagUnique(db *gorm.DB, currentID ctype.ID, title string) error {
	var count int64
	if err := db.Model(&models.TagModel{}).
		Where("title = ? AND id <> ?", title, currentID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("标签名称重复")
	}
	return nil
}
