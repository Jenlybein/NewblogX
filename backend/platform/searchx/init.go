package searchx

import (
	"myblogx/conf"
	"myblogx/core"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
)

// Init 负责初始化 ES 客户端连接。
func Init(esConf *conf.ES, logger *logrus.Logger) *elasticsearch.Client {
	return core.EsConnect(esConf, logger)
}
