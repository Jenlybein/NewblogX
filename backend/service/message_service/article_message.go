package message_service

import (
	"myblogx/models"
	"myblogx/models/ctype"

	"myblogx/models/enum/message_enum"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 插入一条文章评论消息
func InsertCommentMessage(db *gorm.DB, logger *logrus.Logger, content ArticleCommentMessage) {
	if db == nil {
		return
	}
	// if content.ReceiverID == content.ActionUserID {
	// 	return
	// }

	nickname, avatar := getActionUserInfo(db, logger, content.ActionUserID)

	if err := db.Create(&models.ArticleMessageModel{
		Type:               message_enum.CommentArticleType,
		ReceiverID:         content.ReceiverID,
		ActionUserID:       &content.ActionUserID,
		ActionUserNickname: &nickname,
		ActionUserAvatar:   &avatar,

		Content: content.Content,

		ArticleID:    content.ArticleID,
		ArticleTitle: content.ArticleTitle,
		CommentID:    content.CommentID,
	}).Error; err != nil {
		logMessageError(logger, "创建评论消息失败: %v", err)
		return
	}
}

// 插入一条文章评论的回复消息
func InsertReplyMessage(db *gorm.DB, logger *logrus.Logger, content ArticleReplyMessage) {
	if db == nil {
		return
	}
	// if content.ReceiverID == content.ActionUserID {
	// 	return
	// }

	nickname, avatar := getActionUserInfo(db, logger, content.ActionUserID)

	if err := db.Create(&models.ArticleMessageModel{
		Type:               message_enum.CommentReplyType,
		ReceiverID:         content.ReceiverID,
		ActionUserID:       &content.ActionUserID,
		ActionUserNickname: &nickname,
		ActionUserAvatar:   &avatar,

		Content: content.Content,

		ArticleID:    content.ArticleID,
		ArticleTitle: content.ArticleTitle,
		CommentID:    content.CommentID,
	}).Error; err != nil {
		logMessageError(logger, "创建回复消息失败: %v", err)
		return
	}
}

// 插入一条文章点赞消息
func InsertArticleDiggMessage(db *gorm.DB, logger *logrus.Logger, content ArticleDiggMessage) {
	if db == nil {
		return
	}
	// if content.ReceiverID == content.ActionUserID {
	// 	return
	// }

	if err := db.Take(&models.ArticleMessageModel{}, "action_user_id = ? and type = ? and article_id = ?", content.ActionUserID, message_enum.DiggArticleType, content.ArticleID).Error; err == nil {
		return
	}

	nickname, avatar := getActionUserInfo(db, logger, content.ActionUserID)

	if err := db.Create(&models.ArticleMessageModel{
		Type:               message_enum.DiggArticleType,
		ReceiverID:         content.ReceiverID,
		ActionUserID:       &content.ActionUserID,
		ActionUserNickname: &nickname,
		ActionUserAvatar:   &avatar,
		ArticleID:          content.ArticleID,
		ArticleTitle:       content.ArticleTitle,
	}).Error; err != nil {
		logMessageError(logger, "创建文章点赞消息失败: %v", err)
		return
	}
}

// 插入一条评论点赞消息
func InsertCommentDiggMessage(db *gorm.DB, logger *logrus.Logger, content CommentDiggMessage) {
	if db == nil {
		return
	}
	// if content.ReceiverID == content.ActionUserID {
	// 	return
	// }

	if err := db.Take(&models.ArticleMessageModel{}, "action_user_id = ? and type = ? and comment_id = ?", content.ActionUserID, message_enum.DiggCommentType, content.CommentID).Error; err == nil {
		return
	}

	nickname, avatar := getActionUserInfo(db, logger, content.ActionUserID)

	if err := db.Create(&models.ArticleMessageModel{
		Type:               message_enum.DiggCommentType,
		CommentID:          content.CommentID,
		Content:            content.Content,
		ReceiverID:         content.ReceiverID,
		ActionUserID:       &content.ActionUserID,
		ActionUserNickname: &nickname,
		ActionUserAvatar:   &avatar,
		ArticleID:          content.ArticleID,
		ArticleTitle:       content.ArticleTitle,
	}).Error; err != nil {
		logMessageError(logger, "创建评论点赞消息失败: %v", err)
		return
	}
}

// 插入一条文章收藏消息
func InsertArticleFavorMessage(db *gorm.DB, logger *logrus.Logger, content ArticleFavorMessage) {
	if db == nil {
		return
	}
	// if content.ReceiverID == content.ActionUserID {
	// 	return
	// }

	if err := db.Take(&models.ArticleMessageModel{}, "action_user_id = ? and type = ? and article_id = ?", content.ActionUserID, message_enum.FavorArticleType, content.ArticleID).Error; err == nil {
		return
	}

	nickname, avatar := getActionUserInfo(db, logger, content.ActionUserID)

	if err := db.Create(&models.ArticleMessageModel{
		Type:               message_enum.FavorArticleType,
		ReceiverID:         content.ReceiverID,
		ActionUserID:       &content.ActionUserID,
		ActionUserNickname: &nickname,
		ActionUserAvatar:   &avatar,
		ArticleID:          content.ArticleID,
		ArticleTitle:       content.ArticleTitle,
	}).Error; err != nil {
		logMessageError(logger, "创建评论收藏消息失败: %v", err)
		return
	}
}

// 插入一条系统消息
func InsertSystemMessage(db *gorm.DB, logger *logrus.Logger, content SystemMessage) {
	if db == nil {
		return
	}
	if content.ReceiverID == 0 {
		logMessageError(logger, "创建系统消息失败: 接收者ID不能为空")
		return
	}

	msg := models.ArticleMessageModel{
		Type:       message_enum.SystemType,
		ReceiverID: content.ReceiverID,
		Content:    content.Content,
		LinkTitle:  content.LinkTitle,
		LinkHerf:   content.LinkHerf,
	}

	if content.ActionUserID != nil {
		msg.ActionUserID = content.ActionUserID
		nickname, avatar := getActionUserInfo(db, logger, *content.ActionUserID)
		msg.ActionUserNickname = &nickname
		msg.ActionUserAvatar = &avatar
	}

	if err := db.Create(&msg).Error; err != nil {
		logMessageError(logger, "创建系统消息失败: %v", err)
		return
	}
}

func getActionUserInfo(db *gorm.DB, logger *logrus.Logger, actionUserID ctype.ID) (nickname string, avatar string) {
	info := models.UserModel{}
	if err := db.Select("nickname", "avatar").Take(&info, "id = ?", actionUserID).Error; err != nil {
		logMessageError(logger, "获取用户信息失败: %v", err)
		return
	}

	return info.Nickname, info.Avatar
}

func logMessageError(logger *logrus.Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Errorf(format, args...)
}
