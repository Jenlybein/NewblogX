package image_api

import (
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/image_ref_river_service"
	"myblogx/service/image_service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h ImageApi) ImageRemoveView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[models.IDListRequest](c)

	var list []models.ImageModel
	if err := app.DB.Find(&list, "id IN ?", cr.IDList).Error; err != nil {
		res.FailWithError(err, c)
		return
	}
	if len(list) == 0 {
		res.FailWithMsg("删除失败，图片不存在", c)
		return
	}

	for _, item := range list {
		if err := image_service.DeleteObject(image_service.NewDeps(app.QiNiu, app.Upload, app.DB, app.Redis, app.Logger), item.Bucket, item.ObjectKey); err != nil {
			res.FailWithMsg(fmt.Sprintf("删除七牛对象失败: %v", err), c)
			return
		}
	}

	imageIDs := make([]ctype.ID, 0, len(list))
	for _, item := range list {
		imageIDs = append(imageIDs, item.ID)
	}
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := image_ref_river_service.DeleteImageRefsByImageIDs(tx, imageIDs); err != nil {
			return err
		}
		return tx.Unscoped().Delete(&list).Error
	}); err != nil {
		res.FailWithError(err, c)
		return
	}

	msg := fmt.Sprintf("操作成功，删除了 %d 张图片", len(list))
	res.OkWithData(msg, c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName:  "image_remove",
		TargetType:  "image",
		Success:     true,
		Message:     msg,
		RequestBody: map[string]any{"id_list": cr.IDList},
		ResponseBody: map[string]any{
			"deleted_count": len(list),
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}
