package flags

import (
	"fmt"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/es_service"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func FlagESIndex(deps Deps) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "初始化索引失败: ES 客户端未初始化")
	}
	article := models.ArticleModel{}
	index := models.ResolveArticleESIndex(deps.ESIndex)
	pipelineName := article.PipelineName()

	fmt.Printf("正在初始化索引: %s\n", index)
	if err := es_service.CreateIndexForce(deps.ESClient, index, article.Mapping()); err != nil {
		logESError(deps.Logger, "初始化索引失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "索引 %s 初始化成功", index)

	fmt.Printf("正在初始化流水线: %s\n", pipelineName)
	if err := es_service.CreatePipelineForce(deps.ESClient, pipelineName, article.Pipeline()); err != nil {
		logESError(deps.Logger, "初始化流水线失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "流水线 %s 初始化成功", pipelineName)
	return nil
}

// FlagESDelete 非交互地删除 ES 索引和流水线。
func FlagESDelete(deps Deps) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "删除索引失败: ES 客户端未初始化")
	}
	article := models.ArticleModel{}
	index := models.ResolveArticleESIndex(deps.ESIndex)
	pipelineName := article.PipelineName()

	fmt.Printf("正在删除索引: %s\n", index)
	if err := es_service.DeleteIndex(deps.ESClient, index); err != nil {
		logESError(deps.Logger, "删除索引失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "索引 %s 删除成功", index)

	fmt.Printf("正在删除流水线: %s\n", pipelineName)
	if err := es_service.DeletePipeline(deps.ESClient, pipelineName); err != nil {
		logESError(deps.Logger, "删除流水线失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "流水线 %s 删除成功", pipelineName)
	return nil
}

// FlagESEnsure 非交互地确保 ES 索引和流水线存在。
func FlagESEnsure(deps Deps) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "确保索引失败: ES 客户端未初始化")
	}
	article := models.ArticleModel{}
	index := models.ResolveArticleESIndex(deps.ESIndex)
	pipelineName := article.PipelineName()

	if err := es_service.EnsureIndex(deps.ESClient, index, article.Mapping()); err != nil {
		logESError(deps.Logger, "确保索引失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "索引 %s 已就绪", index)

	if err := es_service.EnsurePipeline(deps.ESClient, pipelineName, article.Pipeline()); err != nil {
		logESError(deps.Logger, "确保流水线失败: %v", err)
		return err
	}
	logESInfo(deps.Logger, "流水线 %s 已就绪", pipelineName)
	return nil
}

// FlagESArticleSync 全量同步文章数据到 ES。
func FlagESArticleSync(deps Deps) error {
	if deps.DB == nil {
		return logESError(deps.Logger, "文章同步失败: 数据库未初始化")
	}
	if deps.ESClient == nil {
		return logESError(deps.Logger, "文章同步失败: ES 客户端未初始化")
	}

	index := models.ResolveArticleESIndex(deps.ESIndex)
	exists, err := es_service.ExistsIndex(deps.ESClient, index)
	if err != nil {
		return logESError(deps.Logger, "文章同步失败: 检查 ES 索引失败: %v", err)
	}
	if !exists {
		return logESError(deps.Logger, "文章同步失败: ES 索引 %s 不存在，请先执行 -es -s init", index)
	}

	batchSize := 128
	if deps.RiverConfig.BulkSize > 0 {
		batchSize = deps.RiverConfig.BulkSize
	}

	total, err := syncArticleDocuments(deps.DB, deps.ESClient, index, batchSize, deps.Logger)
	if err != nil {
		return logESError(deps.Logger, "文章同步失败: %v", err)
	}
	logESInfo(deps.Logger, "文章同步完成，共同步 %d 篇文章到索引 %s", total, index)
	return nil
}

// syncArticleDocuments 全量同步文章数据到 ES
func syncArticleDocuments(db *gorm.DB, esClient *elasticsearch.Client, index string, batchSize int, logger *logrus.Logger) (int, error) {
	_ = index // 目前批量同步仍复用 es_service 既有入口，索引名由 es_service 内部路由。
	articles := make([]models.ArticleModel, 0, batchSize)
	total := 0

	result := db.Model(&models.ArticleModel{}).
		Select("id").
		Order("id asc").
		FindInBatches(&articles, batchSize, func(tx *gorm.DB, batch int) error {
			articleIDs := make([]ctype.ID, 0, len(articles))
			for _, article := range articles {
				articleIDs = append(articleIDs, article.ID)
			}

			if len(articleIDs) == 0 {
				return nil
			}

			if err := es_service.SyncESDocs(db, esClient, articleIDs); err != nil {
				return fmt.Errorf("第 %d 批文章同步到 ES 失败: %w", batch, err)
			}

			total += len(articles)
			logESInfo(logger, "文章同步进度: 第 %d 批完成，本批 %d 篇，累计 %d 篇", batch, len(articles), total)
			return nil
		})
	if result.Error != nil {
		return total, result.Error
	}
	return total, nil
}

func buildArticleESDocument(article models.ArticleModel, adminTop, authorTop bool) map[string]any {
	return es_service.BuildArticleESDocument(article, adminTop, authorTop)
}

func logESInfo(logger *logrus.Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Infof(format, args...)
}

func logESError(logger *logrus.Logger, format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	if logger == nil {
		return err
	}
	logger.Error(err)
	return err
}
