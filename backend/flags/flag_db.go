package flags

import (
	"fmt"
	"myblogx/models"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func FlagDB(db *gorm.DB, logger *logrus.Logger) error {
	if db == nil {
		return fmt.Errorf("数据库迁移失败: 数据库未初始化")
	}

	if logger != nil {
		logger.Info("数据库开始迁移")
	}

	err := db.AutoMigrate(
		&models.UserModel{},
		&models.UserConfModel{},
		&models.UserStatModel{},
		&models.UserSessionModel{},
		&models.UserViewDailyModel{},
		&models.ArticleModel{},
		&models.ArticleAIScoreRecordModel{},
		&models.ArticleReviewTaskModel{},
		&models.ArticleReviewLogModel{},
		&models.TagModel{},
		&models.ArticleTagModel{},
		&models.ArticleDiggModel{},
		&models.CategoryModel{},
		&models.FavoriteModel{},
		&models.UserArticleFavorModel{},
		&models.UserArticleViewHistoryModel{},
		&models.UserTopArticleModel{},
		&models.ImageModel{},
		&models.ImageRefModel{},
		&models.RuntimeSiteConfigModel{},
		&models.CommentModel{},
		&models.BannerModel{},
		&models.GlobalNotifModel{},
		&models.CommentDiggModel{},
		&models.ArticleMessageModel{},
		&models.UserGlobalNotifModel{},
		&models.UserFollowModel{},
		&models.ChatSessionModel{},
		&models.ChatMsgModel{},
		&models.ChatMsgUserStateModel{},
		&models.CdcDeadLetterModel{},
	)
	if err != nil {
		if logger != nil {
			logger.Error("数据库迁移失败", err)
		}
		return err
	}
	// if db.Migrator().HasTable("image_upload_task_models") {
	// 	if err := db.Migrator().DropTable("image_upload_task_models"); err != nil {
	// 		flagLogger.Errorf("删除旧图片上传任务表失败: %v", err)
	// 	}
	// }
	if logger != nil {
		logger.Info("数据库迁移成功")
	}
	return nil
}
