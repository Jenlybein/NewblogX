package follow_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/db_service"
	"myblogx/service/read_service"
	"myblogx/service/user_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 当前登录用户关注其他用户
func (h FollowApi) FollowUserView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)

	claims := jwts.GetClaimsByGin(c)

	if cr.ID == claims.UserID {
		res.FailWithMsg("不能关注自己", c)
		return
	}

	var targetUser models.UserModel
	if err := h.App.DB.Select("id").Take(&targetUser, cr.ID).Error; err != nil {
		res.FailWithMsg("目标用户不存在", c)
		return
	}

	// TODO：考虑每天关注量上限和取关量上限

	createdOrRestored := false
	userMap, err := read_service.LoadUserDisplayMap(h.App.DB, []ctype.ID{claims.UserID, cr.ID})
	if err != nil {
		res.FailWithMsg("查询用户信息失败", c)
		return
	}
	followedUser := userMap[cr.ID]
	fansUser := userMap[claims.UserID]
	err = h.App.DB.Transaction(func(tx *gorm.DB) error {
		// 尝试寻找是否存在软删的关注记录，没有则尝试进行创建
		var createErr error
		createdOrRestored, createErr = db_service.RestoreOrCreateUnique(tx, &models.UserFollowModel{
			FollowedUserID:   cr.ID,
			FollowedNickname: followedUser.Nickname,
			FollowedAvatar:   followedUser.Avatar,
			FollowedAbstract: followedUser.Abstract,
			FansUserID:       claims.UserID,
			FansNickname:     fansUser.Nickname,
			FansAvatar:       fansUser.Avatar,
			FansAbstract:     fansUser.Abstract,
		}, []string{"followed_user_id", "fans_user_id"})
		if createErr != nil {
			return createErr
		}
		if !createdOrRestored {
			return nil
		}

		if err := tx.Model(&models.UserFollowModel{}).
			Where("followed_user_id = ? AND fans_user_id = ?", cr.ID, claims.UserID).
			Updates(map[string]any{
				"followed_nickname": followedUser.Nickname,
				"followed_avatar":   followedUser.Avatar,
				"followed_abstract": followedUser.Abstract,
				"fans_nickname":     fansUser.Nickname,
				"fans_avatar":       fansUser.Avatar,
				"fans_abstract":     fansUser.Abstract,
			}).Error; err != nil {
			return err
		}

		// 关注成功后，增加粉丝数和关注数
		return user_service.StatApplyFollowDelta(tx, claims.UserID, cr.ID, 1)
	})
	if err != nil {
		res.FailWithMsg("关注失败", c)
		return
	}
	if !createdOrRestored {
		res.FailWithMsg("请勿重复关注", c)
		return
	}
	res.OkWithMsg("关注成功", c)
}

// 当前登录用户取消关注其他用户
func (h FollowApi) UnfollowUserView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)

	claims := jwts.GetClaimsByGin(c)

	if cr.ID == claims.UserID {
		res.FailWithMsg("不能取消关注自己", c)
		return
	}

	// 取消关注必须看本次 Delete 是否真正命中了活记录，不能只看删除前查到了什么。
	unfollowed := false
	err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		// 尝试寻找是否存在软删的关注记录，没有则尝试进行删除
		deleteResult := tx.Where(map[string]any{
			"followed_user_id": cr.ID,
			"fans_user_id":     claims.UserID,
		}).Delete(&models.UserFollowModel{})
		if deleteResult.Error != nil {
			return deleteResult.Error
		}
		if deleteResult.RowsAffected == 0 {
			return nil
		}
		unfollowed = true

		// 取消关注后，减少粉丝数和关注数
		return user_service.StatApplyFollowDelta(tx, claims.UserID, cr.ID, -1)
	})
	if err != nil {
		res.FailWithMsg("取消关注失败", c)
		return
	}
	if !unfollowed {
		res.FailWithMsg("尚未关注该用户", c)
		return
	}
	res.OkWithMsg("取消关注成功", c)
}
