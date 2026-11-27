package cli

import (
	"fmt"
	"strings"

	"myblogx/conf"
	"myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// FlagMigrateRuntimeConfig 把 site.login 里按通道拆走的字段搬到各自段（票 票 481）。
//
// 为什么需要一次性迁移：那些键已从 site.Login 结构体里删除，而读取路径只把库里
// 载荷合并到当前结构体上 —— 结构体里没有的键会被当成未知键忽略，管理员当年填的
// 值于是静默失效、回落到默认值。用户明确要求不留兼容层，所以只能搬一次。
//
// apply 为假时**只报告不写库**：这条命令会改写 site 与最多四个目标段的 payload，
// 先看清楚"哪些键要搬、哪些因为目标段已有而被跳过"再动手要便宜得多。
//
// 幂等：第二次执行时 site.login 里已经没有这些键，会打印"无需迁移"并且不写库，
// 所以部署脚本不必判断状态。启动路径里**没有**这一步 —— 迁移只在手动执行时发生。
func FlagMigrateRuntimeConfig(db *gorm.DB, logger *logrus.Logger, apply bool) error {
	if db == nil {
		return fmt.Errorf("运行时配置迁移失败: 数据库未初始化")
	}
	// 迁移只看库里的原始 payload，既不补种缺失段（InitRuntimeConfig 会写库），
	// 也不读默认值文件 —— 传空默认值是刻意的：这条命令不该因为默认值文件缺失
	// 或过期而改变"要搬什么"。
	svc := config.NewRuntimeConfigService(conf.RuntimeDefaults{}, logger, db)
	result, err := svc.MigrateLoginSection(apply)
	if err != nil {
		return fmt.Errorf("运行时配置迁移失败: %w", err)
	}

	for _, line := range config.LoginSectionMigrationReportLines(result, apply) {
		fmt.Println(line)
		if logger != nil {
			// 告警行（"被跳过的键没有生效"）走 Warn，方便按级别筛出来；
			// 其余走 Info —— 与 -filter-coverage 的输出口径一致。
			if strings.HasPrefix(line, "⚠") {
				logger.Warn(line)
			} else {
				logger.Info(line)
			}
		}
	}
	return nil
}
