package metainfo

import (
	"context"
	"strings"

	"myblogx/conf"
	"myblogx/internal/ai/domain"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"

	"github.com/sirupsen/logrus"
)

// GenerateArticleMetainfo 根据文章内容生成标题、摘要、分类和标签建议。
//
// 分类/标签候选由调用方通过 Catalog 提供，本包只负责提示词与结果校验。
func GenerateArticleMetainfo(ctx context.Context, catalog domain.Catalog, logger *logrus.Logger, aiConf conf.AI, userID uint64, content string) (*MetainfoResponse, error) {
	if userID == 0 {
		return nil, apperr.New(apperr.CodeAIUserRequired)
	}
	if strings.TrimSpace(content) == "" {
		return nil, apperr.New(apperr.CodeAIContentRequired)
	}
	if catalog == nil {
		return nil, apperr.New(apperr.CodeAIConfigMissing)
	}

	categoryOptions, err := loadOptions(func() ([]domain.Option, error) {
		return catalog.Categories(ctx, userID)
	}, "查询文章分类候选失败", logger)
	if err != nil {
		return nil, err
	}
	tagOptions, err := loadOptions(func() ([]domain.Option, error) {
		return catalog.Tags(ctx)
	}, "查询文章标签候选失败", logger)
	if err != nil {
		return nil, err
	}

	plainText := cleanArticleMetainfoContent(content, aiConf.MaxInputChars)
	if plainText == "" {
		return nil, apperr.New(apperr.CodeAIContentEmpty)
	}

	reply, err := requestArticleMetainfoFromAI(aiConf, plainText, categoryOptions, tagOptions)
	if err != nil {
		return nil, err
	}

	return normalizeArticleMetainfoReply(reply, categoryOptions, tagOptions)
}

// loadOptions 统一处理候选加载的错误与日志。
func loadOptions(load func() ([]domain.Option, error), what string, logger *logrus.Logger) ([]Metainfos, error) {
	options, err := load()
	if err != nil {
		if logger != nil {
			logger.Errorf("%s: %v", what, err)
		}
		return nil, err
	}
	out := make([]Metainfos, 0, len(options))
	for _, option := range options {
		out = append(out, Metainfos{ID: ctype.NewID(option.ID.Uint64()), Title: option.Title})
	}
	return out, nil
}
