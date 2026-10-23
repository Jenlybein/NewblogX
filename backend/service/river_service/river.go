package river_service

import (
	"context"
	"fmt"
	"myblogx/conf"
	"myblogx/service/log_service"
	"myblogx/service/river_service/rule"
	"regexp"
	"strings"
	"sync"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/pingcap/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ErrRuleNotExist 是规则不存在的错误
var ErrRuleNotExist = errors.New("规则不存在")

// River 是一个可插拔的服务，它从Elasticsearch中拉取数据然后将其索引到Elasticsearch中。
type River struct {
	canal *canal.Canal // MySQL的canal实例

	rules   map[string]*rule.Rule // 规则映射
	cfg     conf.River
	log     *logrus.Logger
	logDeps log_service.Deps
	db      *gorm.DB
	es      *elasticsearch.Client

	ctx    context.Context    // 上下文
	cancel context.CancelFunc // 取消函数

	wg sync.WaitGroup // 等待组

	master *masterInfo // 主库信息

	syncCh chan interface{} // 同步通道
}

func (r *River) SetLogDeps(deps log_service.Deps) {
	r.logDeps = deps
}

// NewRiver 根据配置创建 River 实例
func NewRiver(config conf.River, logger *logrus.Logger, db *gorm.DB, esClient *elasticsearch.Client) (*River, error) {
	r := &River{
		rules:  make(map[string]*rule.Rule),
		cfg:    config,
		log:    logger,
		db:     db,
		es:     esClient,
		syncCh: make(chan interface{}, 4096),
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())

	var err error
	if r.master, err = loadMasterInfo(r.cfg.DataDir, r.log); err != nil {
		return nil, errors.Trace(err)
	}

	if err = r.newCanal(); err != nil {
		return nil, errors.Trace(err)
	}

	if err = r.prepareRule(); err != nil {
		return nil, errors.Trace(err)
	}

	if err = r.prepareCanal(); err != nil {
		return nil, errors.Trace(err)
	}

	// We must use binlog full row image
	if err = r.canal.CheckBinlogRowImage("FULL"); err != nil {
		return nil, errors.Trace(err)
	}

	return r, nil
}

// newCanal 创建canal实例
func (r *River) newCanal() error {
	cfg := canal.NewDefaultConfig()

	// 配置日志记录器
	cfg.Logger = logrusToSlogAdapter(r.log)

	// 配置mysql连接信息
	cfg.Addr = r.cfg.Mysql.Addr
	cfg.User = r.cfg.Mysql.User
	cfg.Password = r.cfg.Mysql.Password
	cfg.Charset = r.cfg.Charset
	cfg.Flavor = r.cfg.Flavor

	cfg.ServerID = r.cfg.ServerID
	cfg.Dump.ExecutionPath = r.cfg.DumpExec
	cfg.Dump.DiscardErr = false
	cfg.Dump.SkipMasterData = r.cfg.SkipMasterData

	// 配置需要同步的数据库表，添加正则表达式 "schema\\.table"
	for _, s := range r.cfg.Sources {
		for _, t := range s.Tables {
			cfg.IncludeTableRegex = append(cfg.IncludeTableRegex, s.Schema+"\\."+t)
		}
	}

	var err error
	r.canal, err = canal.NewCanal(cfg)
	return errors.Trace(err)
}

// prepareCanal 准备canal实例
func (r *River) prepareCanal() error {
	var db string
	dbs := map[string]struct{}{}
	tables := make([]string, 0, len(r.rules))
	for _, rule := range r.rules {
		db = rule.Schema
		dbs[rule.Schema] = struct{}{}
		tables = append(tables, rule.Table)
	}

	if len(dbs) == 1 {
		// one db, we can shrink using table
		r.canal.AddDumpTables(db, tables...)
	} else {
		// many dbs, can only assign databases to dump
		keys := make([]string, 0, len(dbs))
		for key := range dbs {
			keys = append(keys, key)
		}

		r.canal.AddDumpDatabases(keys...)
	}

	r.canal.SetEventHandler(&eventHandler{r})

	return nil
}

// newRule 创建新规则
func (r *River) newRule(schema, table string) error {
	key := ruleKey(schema, table)

	if _, ok := r.rules[key]; ok {
		return errors.Errorf("重复的数据源 %s, %s 已在配置中定义", schema, table)
	}

	r.rules[key] = rule.NewDefaultRule(schema, table)
	return nil
}

// updateRule 更新规则
func (r *River) updateRule(schema, table string) error {
	rule, ok := r.rules[ruleKey(schema, table)]
	if !ok {
		return ErrRuleNotExist
	}

	tableInfo, err := r.canal.GetTable(schema, table)
	if err != nil {
		return errors.Trace(err)
	}

	rule.TableInfo = tableInfo

	return nil
}

// parseSource 解析数据源
func (r *River) parseSource() (map[string][]string, error) {
	// 存储通配符表的映射关系，key为 schema.table，value为匹配的表名列表
	wildTables := make(map[string][]string, len(r.cfg.Sources))

	// 解析数据源，获取通配符表的映射关系
	for _, s := range r.cfg.Sources {
		if !isValidTables(s.Tables) {
			return nil, errors.Errorf("不允许在多个表中使用通配符 *")
		}

		// 检查数据库名是否为空
		if len(s.Schema) == 0 {
			return nil, errors.Errorf("数据源中不允许为空的数据库名")
		}

		// 解析各个表
		for _, table := range s.Tables {
			// 检查表名是否包含正则表达式特殊字符（即是否为通配符表）
			if regexp.QuoteMeta(table) != table {
				if _, ok := wildTables[ruleKey(s.Schema, table)]; ok {
					return nil, errors.Errorf("数据源中定义了重复的通配符表 %s.%s", s.Schema, table)
				}

				// 执行查询获取匹配的表名，将结果存储在tables切片中
				tables := []string{} // 存储匹配的表名列表（通配符存在导致可能有多个匹配）

				sql := fmt.Sprintf(`SELECT table_name FROM information_schema.tables WHERE
					table_name RLIKE "%s" AND table_schema = "%s";`, buildTable(table), s.Schema)

				res, err := r.canal.Execute(sql)
				if err != nil {
					return nil, errors.Trace(err)
				}

				for i := 0; i < res.Resultset.RowNumber(); i++ {
					f, _ := res.GetString(i, 0)
					err := r.newRule(s.Schema, f)
					if err != nil {
						return nil, errors.Trace(err)
					}

					tables = append(tables, f)
				}

				wildTables[ruleKey(s.Schema, table)] = tables
			} else {
				err := r.newRule(s.Schema, table)
				if err != nil {
					return nil, errors.Trace(err)
				}
			}
		}
	}

	if len(r.rules) == 0 {
		return nil, errors.Errorf("未定义可同步的数据源")
	}

	return wildTables, nil
}

// prepareRule 准备规则 - 初始化和配置用于数据同步的规则
func (r *River) prepareRule() error {
	// 解析数据源，获取通配符表的映射关系
	wildtables, err := r.parseSource()
	if err != nil {
		return errors.Trace(err)
	}

	// 如果配置了自定义规则，则应用这些规则
	if r.cfg.Rules != nil {
		// 遍历所有自定义规则
		for _, rule := range r.cfg.Rules {
			// 检查规则的数据库名是否为空
			if len(rule.Schema) == 0 {
				return errors.Errorf("自定义规则中不允许为空的数据库名")
			}

			// 检查表名是否包含正则表达式特殊字符（即是否为通配符表）
			if regexp.QuoteMeta(rule.Table) != rule.Table {
				// 处理通配符表的情况
				tables, ok := wildtables[ruleKey(rule.Schema, rule.Table)]
				if !ok {
					return errors.Errorf("通配符表 %s.%s 在数据源中未定义", rule.Schema, rule.Table)
				}

				// 通配符规则必须指定索引名称
				if len(rule.Index) == 0 {
					return errors.Errorf("通配符表规则 %s.%s 必须指定索引名称，不能为空", rule.Schema, rule.Table)
				}

				// 准备规则（预处理操作）
				rule.Prepare()

				// 将当前规则的配置应用到所有匹配的表上
				for _, table := range tables {
					// 获取对应表的规则对象
					rr := r.rules[ruleKey(rule.Schema, table)]
					// 应用索引、类型、父级等配置
					rr.Index = rule.Index
					rr.Type = rule.Type
					rr.Parent = rule.Parent
					rr.ID = rule.ID
					rr.FieldMapping = rule.FieldMapping
				}
			} else {
				// 处理非通配符表（精确匹配）的情况
				key := ruleKey(rule.Schema, rule.Table)
				// 检查该表是否已在源配置中定义
				if _, ok := r.rules[key]; !ok {
					return errors.Errorf("规则 %s.%s 在数据源中未定义", rule.Schema, rule.Table)
				}
				// 准备规则
				rule.Prepare()
				// 替换原有的默认规则为自定义规则
				r.rules[key] = rule
			}
		}
	}

	// 创建新的规则映射，过滤掉没有主键的表
	rules := make(map[string]*rule.Rule)
	for key, rule := range r.rules {
		// 从MySQL获取表结构信息
		if rule.TableInfo, err = r.canal.GetTable(rule.Schema, rule.Table); err != nil {
			return errors.Trace(err)
		}

		// 检查表是否有主键，没有主键的表会被忽略（因为无法进行有效的同步）
		if len(rule.TableInfo.PKColumns) == 0 {
			if r.log != nil {
				r.log.Errorf("忽略未配置主键的数据表: %s", rule.TableInfo.Name)
			}
		} else {
			// 只保留有主键的表规则
			rules[key] = rule
		}
	}
	// 更新规则映射
	r.rules = rules

	return nil
}

// ruleKey 生成规则键
func ruleKey(schema string, table string) string {
	return strings.ToLower(fmt.Sprintf("%s:%s", schema, table))
}

// Run 从MySQL同步数据并插入到ES中
func (r *River) Run() error {
	r.wg.Add(1)
	go r.syncLoop()

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

// Close 关闭River
func (r *River) Close() {
	if r.log != nil {
		r.log.Infof("开始关闭 River 同步服务")
	}

	r.cancel()

	r.canal.Close()

	r.master.Close()

	r.wg.Wait()
}

// isValidTables 检查表名是否有效
func isValidTables(tables []string) bool {
	if len(tables) > 1 {
		for _, table := range tables {
			if table == "*" {
				return false
			}
		}
	}
	return true
}

// buildTable 构建表名
func buildTable(table string) string {
	if table == "*" {
		return "." + table
	}
	return table
}
