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
		// 孤儿图候选清单（#139）。本票只读，不提供删除；-apply 留给 #140。
		if err := FlagImageGCOrphans(deps.DB, deps.Logger, op.File, op.Apply); err != nil {
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
