package river

// River：MySQL binlog → 文章搜索投影。
//
// # 它现在只负责"接线"
//
// canal 连上 MySQL、把行事件交给 eventHandler；真正的翻译（哪张表变了 → 要重算哪些文档）
// 在 article_search_projection.go，拼文档写 ES 在 articlesync 包里。
//
// # 关于被删掉的"通用规则"路径
//
// 这里原来有一套 `rule` 机制：从配置读 `表 → 索引 + 字段白名单`，把行事件一行一行搬进 ES。
// 2026-10-26 整条删除 —— 它对本项目配置里的每一张表都不可达，而且拼不出搜索文档。
// 删除依据与代价见 ADR-008 与 sync.go 的包注释。
//
// 保留的内容里，`master`（位点）与 canal 的 dump 配置仍然必要：全量转储会把表里的行
// 按 INSERT 重放一遍，投影分支会照常把它们变成 ES 文档 —— 这就是首次部署的数据初始化。

import (
	"context"
	"strings"
	"sync"

	"myblogx/conf"
	"myblogx/internal/platform/cdc/canalbase"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/safego"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/pingcap/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// River 从 MySQL 的 binlog 增量维护文章搜索投影。
type River struct {
	canal *canal.Canal // MySQL 的 canal 实例

	cfg        conf.River
	log        *logrus.Logger
	logOptions writer.Options
	db         *gorm.DB
	es         *elasticsearch.Client

	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	master *masterInfo // 位点

	syncCh chan posSaver // 位点落盘通道
}

// SetLogOptions 注入日志选项（装配层调用）。
func (r *River) SetLogOptions(deps writer.Options) {
	r.logOptions = deps
}

// NewRiver 根据配置创建 River 实例
func NewRiver(config conf.River, logger *logrus.Logger, db *gorm.DB, esClient *elasticsearch.Client) (*River, error) {
	r := &River{
		cfg:    config,
		log:    logger,
		db:     db,
		es:     esClient,
		syncCh: make(chan posSaver, 4096),
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())

	var err error
	if r.master, err = loadMasterInfo(r.cfg.DataDir, r.log); err != nil {
		return nil, errors.Trace(err)
	}

	if err = r.newCanal(); err != nil {
		return nil, errors.Trace(err)
	}

	r.prepareCanal()

	// 必须拿到完整行镜像：投影要靠行里的列值算文章 ID / 标签关联 / 置顶状态。
	if err = r.canal.CheckBinlogRowImage("FULL"); err != nil {
		return nil, errors.Trace(err)
	}

	return r, nil
}

// newCanal 创建 canal 实例（监听范围来自配置的 Sources）
func (r *River) newCanal() error {
	listen := canalbase.Spec{Tables: map[string][]string{}}
	for _, s := range r.cfg.Sources {
		listen.Tables[s.Schema] = append(listen.Tables[s.Schema], s.Tables...)
	}

	c, err := canalbase.Build(canalbase.MySQL{
		Addr:           r.cfg.Mysql.Addr,
		User:           r.cfg.Mysql.User,
		Password:       r.cfg.Mysql.Password,
		Charset:        r.cfg.Charset,
		Flavor:         r.cfg.Flavor,
		ServerID:       r.cfg.ServerID,
		DumpExec:       r.cfg.DumpExec,
		SkipMasterData: r.cfg.SkipMasterData,
	}, listen, r.log)
	if err != nil {
		return errors.Trace(err)
	}

	r.canal = c
	return nil
}

// prepareCanal 注册事件处理器并设置全量转储范围。
//
// 转储范围直接取配置的 Sources（以前绕了一圈从 rule 里取，rule 删掉后没必要了），
// 但要排除 outbox：转储会把表里所有历史行按 INSERT 重放一遍，
// 而那些行绝大多数早就投递完成（原子领取会挡掉，但仍是无谓的全表扫描）。
// 它的积压恢复交给中继自己的补偿扫描（有 LIMIT、按 id 升序，可控）。
func (r *River) prepareCanal() {
	dump := canalbase.Spec{Tables: map[string][]string{}}
	for _, s := range r.cfg.Sources {
		for _, table := range s.Tables {
			if strings.EqualFold(table, outboxrelay.OutboxTableName) {
				continue
			}
			dump.Tables[s.Schema] = append(dump.Tables[s.Schema], table)
		}
	}

	canalbase.PrepareDump(r.canal, dump)
	r.canal.SetEventHandler(&eventHandler{r})
}

// Run 从上次位点开始消费 binlog。
func (r *River) Run() error {
	r.wg.Add(1)
	safego.Go(func(recovered any, stack []byte) {
		if r.log != nil {
			r.log.Errorf("同步循环 panic: %v\n%s", recovered, stack)
		}
	}, r.syncLoop)

	pos := r.master.Position()
	if err := r.canal.RunFrom(pos); err != nil {
		if r.log != nil {
			r.log.Errorf("启动 Canal 同步失败: %v", err)
		}
		return errors.Trace(err)
	}

	return nil
}

// Ctx 返回内部上下文供外部使用
func (r *River) Ctx() context.Context {
	return r.ctx
}

// Close 关闭 River
func (r *River) Close() {
	if r.log != nil {
		r.log.Infof("开始关闭 River 同步服务")
	}

	r.cancel()

	r.canal.Close()

	r.master.Close()

	r.wg.Wait()
}
