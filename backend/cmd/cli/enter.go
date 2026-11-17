// flags/enter.go
package cli

import (
	"flag"
	"fmt"
	"myblogx/conf"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type FlagOptions struct {
	/* 定义命令行参数选项结构体,用于存储和处理命令行传入的各种标志参数 */
	File               string
	DB                 bool
	Version            bool
	Type               string
	Sub                string
	ES                 bool
	Role               string
	UserCreateRole     string
	UserCreateUsername string
	UserCreatePassword string
	UserCreateNickname string
	UserCreateEmail    string
	UserNoPrompt       bool
	// Apply 让"对账类"子命令真正落库。默认只读，不带它就只统计不改。
	Apply bool
	// EncryptConfig 把库里已存的明文密钥一次性加密（票 359）。
	EncryptConfig bool
	// FilterImport 是初始词表导入文件（TSV/NDJSON，票 395）。
	FilterImport string
	// FilterImportApply 才真正写库；默认 dry-run 只报告。
	FilterImportApply bool
	// FilterSweep 是存量内容清洗（票 402）：扫六个文本面，默认 dry-run 只报告。
	FilterSweep bool
	// FilterSweepApply 才真正写库（替换文本 + 补写关键词风险记录）。
	FilterSweepApply bool
	// FilterSweepReindex 让清洗完成后顺带重建 ES 索引（-es -s init + article-sync）。
	FilterSweepReindex bool
}

type Options struct {
	RiverConfig conf.River
	Logger      *logrus.Logger
	DB          *gorm.DB
	ESClient    *elasticsearch.Client
	ESIndex     string
}

type RunResult struct {
	ContinueStartup bool
}

func Parse() *FlagOptions {
	var Flags = new(FlagOptions)

	flag.StringVar(&Flags.File, "f", "config/settings.yaml", "指定配置文件路径")
	flag.BoolVar(&Flags.DB, "db", false, "数据库迁移")
	flag.BoolVar(&Flags.Version, "version", false, "显示版本信息")
	flag.StringVar(&Flags.Type, "t", "", "操作类型")
	flag.StringVar(&Flags.Sub, "s", "", "子操作类型")
	flag.BoolVar(&Flags.ES, "es", false, "初始化ES索引")
	flag.StringVar(&Flags.Role, "role", "all", "启动角色: api|river|image-ref|cron|worker|all")
	flag.StringVar(&Flags.UserCreateRole, "user-role", "", "创建用户角色: admin|user|guest 或 1|2|3")
	flag.StringVar(&Flags.UserCreateUsername, "user-username", "", "创建用户用户名")
	flag.StringVar(&Flags.UserCreatePassword, "user-password", "", "创建用户密码")
	flag.StringVar(&Flags.UserCreateNickname, "user-nickname", "命令用户", "创建用户昵称")
	flag.StringVar(&Flags.UserCreateEmail, "user-email", "", "创建用户邮箱(可选)")
	flag.BoolVar(&Flags.UserNoPrompt, "user-no-prompt", false, "创建用户时禁用交互输入(参数不完整则直接失败)")
	flag.BoolVar(&Flags.Apply, "apply", false, "对账类子命令真正落库(默认只读，只统计不改)")
	flag.BoolVar(&Flags.EncryptConfig, "encrypt-config", false, "把已存的明文密钥加密(需要环境变量 BLOGX_CONFIG_SECRET_KEY)")
	flag.StringVar(&Flags.FilterImport, "filter-import", "", "从文件导入初始词表(TSV: 词<TAB>档位<TAB>类目，或 NDJSON)，默认只报告不写库")
	flag.BoolVar(&Flags.FilterImportApply, "filter-import-apply", false, "初始词表导入真正写库(默认 dry-run)")
	flag.BoolVar(&Flags.FilterSweep, "filter-sweep", false, "存量内容清洗: 扫描六个文本面的命中词，默认只报告不写库(需先导入词表)")
	flag.BoolVar(&Flags.FilterSweepApply, "filter-sweep-apply", false, "存量清洗真正写库(替换为等长 * + 补写关键词风险记录)")
	flag.BoolVar(&Flags.FilterSweepReindex, "filter-sweep-reindex", false, "清洗完成后顺带重建 ES 索引(等价于 -es -s init + -es -s article-sync)")

	flag.Parse()

	return Flags
}

func Run(op *FlagOptions, deps Options) (RunResult, error) {
	if op.DB {
		// 执行数据库迁移
		if err := FlagDB(deps.DB, deps.Logger); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	}

	if op.EncryptConfig {
		if err := FlagEncryptConfig(deps.DB, deps.Logger); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	}

	if op.FilterImport != "" || op.FilterImportApply {
		// 初始词表导入（#395）：不给文件就等于没指定要导入什么，宁可报错也不要
		// 静默继续启动 —— 那会让人误以为词表已经导进去了。
		if op.FilterImport == "" {
			return RunResult{}, fmt.Errorf("-filter-import-apply 需要配合 -filter-import <file> 使用")
		}
		if err := FlagFilterImport(deps.DB, deps.Logger, op.File, op.FilterImport, op.FilterImportApply); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	}

	if op.FilterSweep || op.FilterSweepApply || op.FilterSweepReindex {
		// 存量清洗（#402）：依赖 -filter-sweep 这个"我要清洗"的显式开关，光带
		// -filter-sweep-apply 不能猜出意图（与 -filter-import 的接线同一口径）。
		if !op.FilterSweep {
			return RunResult{}, fmt.Errorf("-filter-sweep-apply / -filter-sweep-reindex 需要配合 -filter-sweep 使用")
		}
		if err := FlagFilterSweep(deps, op.File, op.FilterSweepApply, op.FilterSweepReindex); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	}

	if op.ES {
		switch op.Sub {
		case "init":
			if err := FlagESIndex(deps); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: false}, nil
		case "delete":
			if err := FlagESDelete(deps); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: false}, nil
		case "ensure":
			if err := FlagESEnsure(deps); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: false}, nil
		case "article-sync":
			if err := FlagESArticleSync(deps); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: false}, nil
		}
		return RunResult{}, fmt.Errorf("未知 ES 子操作类型: %s", op.Sub)
	}

	switch op.Type {
	case "reconcile":
		// 一次性对账（#96）。当前只有 digg 一个子项：
		// 取消点赞留下的软删残行占着唯一键，需要看得见、清得掉。
		switch op.Sub {
		case "digg":
			if err := FlagReconcileDigg(deps.DB, deps.Logger, op.Apply); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: false}, nil
		}
		return RunResult{}, fmt.Errorf("未知对账子类型: %s", op.Sub)
	case "imagegc":
		// 孤儿图：不带 -apply 是只读清单（含二次确认预演），带 -apply 才真删；
		// 真删还受运行时开关 image_gc.enabled 约束。
		if err := FlagImageGCOrphans(deps.DB, deps.Logger, op.File, op.Apply); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	case "imagerefgc":
		// 孤儿图片引用：轮播表重建后，账本里指向已消失/已软删轮播的行。
		// 不带 -apply 只读清单，带 -apply 才删。
		if err := FlagImageRefGCOrphans(deps.DB, deps.Logger, op.Apply); err != nil {
			return RunResult{}, err
		}
		return RunResult{ContinueStartup: false}, nil
	case "run":
		switch op.Sub {
		case "init":
			if err := FlagDB(deps.DB, deps.Logger); err != nil {
				return RunResult{}, err
			}
			if err := FlagESEnsure(deps); err != nil {
				return RunResult{}, err
			}
			return RunResult{ContinueStartup: true}, nil
		default:
			return RunResult{}, fmt.Errorf("未知运行子操作类型: %s", op.Sub)
		}
	case "user":
		u := FlagUser{}
		switch op.Sub {
		case "create":
			u.Create(
				deps.DB,
				deps.Logger,
				UserCreateOptions{
					Role:           op.UserCreateRole,
					Username:       op.UserCreateUsername,
					Password:       op.UserCreatePassword,
					Nickname:       op.UserCreateNickname,
					Email:          op.UserCreateEmail,
					NonInteractive: op.UserNoPrompt,
				},
			)
			return RunResult{ContinueStartup: false}, nil
		default:
			return RunResult{}, fmt.Errorf("未知用户子操作类型: %s", op.Sub)
		}
	}

	return RunResult{ContinueStartup: true}, nil
}
