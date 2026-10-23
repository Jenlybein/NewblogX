package ai_metainfo

import (
	"errors"
	"fmt"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"strings"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// GenerateArticleMetainfo 根据文章内容生成标题、摘要、分类和标签建议。
func GenerateArticleMetainfo(db *gorm.DB, logger *logrus.Logger, aiConf conf.AI, uid ctype.ID, content string) (*MetainfoResponse, error) {
	if uid == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("文章内容不能为空")
	}
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}

	// 加载文章分类候选。
	var categoryOptions []Metainfos
	if err := db.Model(&models.CategoryModel{}).
		Where("user_id = ?", uid).
		Order("id asc").
		Select("id", "title").
		Scan(&categoryOptions).Error; err != nil {
		if logger != nil {
			logger.Errorf("查询文章分类候选失败: 用户ID=%d 错误=%v", uid, err)
		}
		return nil, fmt.Errorf("查询分类候选失败: %w", err)
	}

	// 加载文章标签候选。
	var tagOptions []Metainfos
	if err := db.Model(&models.TagModel{}).
		Where("is_enabled = ?", true).
		Order("sort desc, id asc").
		Select("id", "title").
		Scan(&tagOptions).Error; err != nil {
		if logger != nil {
			logger.Errorf("查询文章标签候选失败: 错误=%v", err)
		}
		return nil, fmt.Errorf("查询标签候选失败: %w", err)
	}

	plainText := cleanArticleMetainfoContent(content, aiConf.MaxInputChars)
	if plainText == "" {
		return nil, errors.New("文章正文提取结果为空")
	}

	reply, err := requestArticleMetainfoFromAI(aiConf, plainText, categoryOptions, tagOptions)
	if err != nil {
		return nil, err
	}

	return normalizeArticleMetainfoReply(reply, categoryOptions, tagOptions)
}
