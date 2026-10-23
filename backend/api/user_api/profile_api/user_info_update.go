package profile_api

import (
	"encoding/json"
	"fmt"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/image_service"
	"myblogx/service/read_service"
	"myblogx/utils/info_check"
	"myblogx/utils/jwts"
	"myblogx/utils/maps"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserInfoUpdateRequest struct {
	Username            *string     `json:"username"`
	Nickname            *string     `json:"nickname"`
	AvatarImageID       *ctype.ID   `json:"avatar_image_id"`
	Abstract            *string     `json:"abstract"`
	LikeTagIDs          *[]ctype.ID `json:"like_tag_ids"`
	FavoritesVisibility *bool       `json:"favorites_visibility"`
	FollowVisibility    *bool       `json:"followers_visibility"`
	FansVisibility      *bool       `json:"fans_visibility"`
	HomeStyleID         *ctype.ID   `json:"home_style_id"`
}

func (h ProfileApi) UserInfoUpdateView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindJson[UserInfoUpdateRequest](c)

	userMap, err := maps.FieldsStructToMap(&cr, &models.UserModel{})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	confMap, err := maps.FieldsStructToMap(&cr, &models.UserConfModel{})
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	normalizeUserConfUpdateMap(confMap)

	if cr.LikeTagIDs != nil {
		likeTagIDs := *cr.LikeTagIDs
		tagIDs, err := validateLikeTagIDs(app.DB, likeTagIDs)
		if err != nil {
			res.FailWithMsg(err.Error(), c)
			return
		}
		likeTagsJSON, err := json.Marshal(tagIDs)
		if err != nil {
			res.FailWithMsg("偏好标签格式错误", c)
			return
		}
		confMap["like_tags"] = string(likeTagsJSON)
	}
	if cr.AvatarImageID != nil {
		avatarURL, err := image_service.ResolveImageURLByID(app.DB, *cr.AvatarImageID)
		if err != nil {
			res.FailWithMsg(err.Error(), c)
			return
		}
		userMap["avatar"] = avatarURL
	}

	claims := jwts.MustGetClaimsByGin(c)

	// 处理用户基本表的更新
	if len(userMap) > 0 {
		var userModel models.UserModel
		if err = app.DB.Preload("UserConfModel").Take(&userModel, claims.UserID).Error; err != nil {
			res.FailWithMsg("用户不存在", c)
			return
		}

		if cr.Username != nil && *cr.Username != userModel.Username {
			// 校验用户名格式
			if err = info_check.CheckUsername(*cr.Username); err != nil {
				res.FailWithError(err, c)
				return
			}

			// 校验用户名是否已被使用
			var nameCount int64
			if err = app.DB.Model(&models.UserModel{}).Where("username = ?", *cr.Username).Count(&nameCount).Error; err != nil {
				res.FailWithError(err, c)
				return
			}
			if nameCount > 0 {
				res.FailWithMsg("用户名已被使用", c)
				return
			}

			// 校验用户名更新频率
			uud := userModel.UserConfModel.UpdatedUsernameDate

			if uud != nil {
				updateLimit := time.Hour * 720
				if time.Since(*uud) < updateLimit {
					res.FailWithMsg(fmt.Sprintf("用户名每 %d 天内只能更新 1 次", int(updateLimit.Hours()/24)), c)
					return
				}
			}

			confMap["updated_username_date"] = time.Now()
		}

		if err = app.DB.Model(&userModel).Updates(userMap).Error; err != nil {
			res.FailWithMsg("用户信息更新失败", c)
			return
		}
		if cr.Nickname != nil || cr.AvatarImageID != nil || cr.Abstract != nil {
			if err = read_service.SyncUserDisplaySnapshots(app.DB, claims.UserID); err != nil {
				app.Logger.Errorf("同步用户展示快照失败: 用户ID=%d 错误=%v", claims.UserID, err)
			}
		}
	}

	// 处理用户配置表的更新
	if len(confMap) > 0 {
		var userConfModel models.UserConfModel
		if err = app.DB.Take(&userConfModel, claims.UserID).Error; err != nil {
			res.FailWithMsg("用户配置信息不存在", c)
			return
		}

		if err = app.DB.Model(&userConfModel).Updates(confMap).Error; err != nil {
			res.FailWithMsg("用户配置信息更新失败", c)
			return
		}
	}

	res.OkWithMsg("用户信息更新成功", c)
}

func validateLikeTagIDs(db *gorm.DB, tagIDs []ctype.ID) ([]ctype.ID, error) {
	normalized := normalizeIDs(tagIDs)
	if len(normalized) == 0 {
		return []ctype.ID{}, nil
	}

	var count int64
	if err := db.Model(&models.TagModel{}).
		Where("id IN ? AND is_enabled = ?", normalized, true).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count != int64(len(normalized)) {
		return nil, fmt.Errorf("偏好标签不存在或已停用")
	}
	return normalized, nil
}

func normalizeIDs(ids []ctype.ID) []ctype.ID {
	result := make([]ctype.ID, 0, len(ids))
	seen := make(map[ctype.ID]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func normalizeUserConfUpdateMap(confMap map[string]any) {
	if confMap == nil {
		return
	}
	// UserConfModel 的数据库列名是 follow_visibility，前端入参是 followers_visibility。
	// 这里统一做一次 key 归一化，避免直接透传 json tag 导致列名不匹配。
	if value, ok := confMap["followers_visibility"]; ok {
		confMap["follow_visibility"] = value
		delete(confMap, "followers_visibility")
	}
}
