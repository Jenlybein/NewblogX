package cli

import (
	"fmt"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/es"
	"myblogx/internal/platform/es/articlesync"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func FlagESIndex(deps Options) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "初始化索引失败: ES 客户端未初始化")
	}
	article := articlesync.ArticleRow{}
	index := articlesync.ResolveIndex(deps.ESIndex)
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
func FlagESDelete(deps Options) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "删除索引失败: ES 客户端未初始化")
	}
	article := articlesync.ArticleRow{}
	index := articlesync.ResolveIndex(deps.ESIndex)
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
func FlagESEnsure(deps Options) error {
	if deps.ESClient == nil {
		return logESError(deps.Logger, "确保索引失败: ES 客户端未初始化")
	}
	article := articlesync.ArticleRow{}
	index := articlesync.ResolveIndex(deps.ESIndex)
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
func FlagESArticleSync(deps Options) error {
	if deps.DB == nil {
		return logESError(deps.Logger, "文章同步失败: 数据库未初始化")
	}
	if deps.ESClient == nil {
		return logESError(deps.Logger, "文章同步失败: ES 客户端未初始化")
	}

	index := articlesync.ResolveIndex(deps.ESIndex)
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

	// 全量重建走的是同一个写入入口，索引名必须一并注入 ——
	// 否则配了自定义索引名时，会"检查的是新索引、写进的是默认索引"。
	articlesync.SetIndexName(index)

	total, err := syncArticleDocuments(deps.DB, deps.ESClient, index, batchSize, deps.Logger)
	if err != nil {
		return logESError(deps.Logger, "文章同步失败: %v", err)
	}
	logESInfo(deps.Logger, "文章同步完成，共同步 %d 篇文章到索引 %s", total, index)
	return nil
}

// syncArticleDocuments 全量同步文章数据到 ES
func syncArticleDocuments(db *gorm.DB, esClient *elasticsearch.Client, index string, batchSize int, logger *logrus.Logger) (int, error) {
	// 索引名不从这里往下传：写入入口统一读 articlesync.IndexName()，
	// 由调用方在开始前用 SetIndexName 注入（见 FlagESArticleSync）。
	// 保留参数是为了让日志与"检查索引存在"沿用同一个名字。
	_ = index
	// 只需要 id：批量同步把 id 交给 articlesync，再由它自己批量装载文档。
	// 用一个最小投影而不是整行，避免为了"拿个 id"把整张表的结构搬进来。
	articles := make([]articleIDRow, 0, batchSize)
	total := 0

	result := db.Model(&articleIDRow{}).
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

			if err := articlesync.SyncESDocs(db, esClient, articleIDs); err != nil {
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

// articleIDRow 是"只选 id"的查询投影。
type articleIDRow struct {
	ID ctype.ID
}

// TableName 固定表名。
func (articleIDRow) TableName() string { return "article_models" }

func buildArticleESDocument(article articlesync.ArticleRow, adminTop, authorTop bool) map[string]any {
	return articlesync.BuildArticleESDocument(article, adminTop, authorTop)
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
