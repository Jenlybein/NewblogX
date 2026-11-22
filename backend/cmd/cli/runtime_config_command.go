package cli

import (
	"fmt"

	"myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// runtimeConfigServiceForCommand 是几条"词表相关"命令共用的启动序列：
// 读默认值（缺字段时补齐）→ 建运行时配置服务 → 载入某一个口径。
//
// 抽出来的原因很实在：导入、清洗、体检三条命令原本各抄一遍这 8 行，只有错
// 误文案里的动作名不同。抄三份的后果不是"难看"，而是容易漏改其中一份 ——
// 体检那次就漏了 readOnly，差点把"只读命令会写库"带上线。
//
// action 是错误文案里的动作名（"初始词表导入" / "存量清洗" / "词表覆盖体检"）；
// readOnly 为真时走 LoadRuntimeConfig（不补默认行 = 不写库），否则走
// InitRuntimeConfig（缺段时补默认行，是启动路径该做的事）。
func runtimeConfigServiceForCommand(
	db *gorm.DB,
	logger *logrus.Logger,
	configFile string,
	action string,
	readOnly bool,
) (*config.RuntimeConfigService, error) {
	defaults, defaultsErr := config.LoadDefaults(configFile)
	if defaultsErr != nil && logger != nil {
		logger.Warnf("读取站点默认配置失败，%s将只用库里那份运行时配置: %v", action, defaultsErr)
	}

	svc := config.NewRuntimeConfigService(defaults, logger, db)
	if readOnly {
		if err := svc.LoadRuntimeConfig(); err != nil {
			return nil, fmt.Errorf("%s失败: 读取运行时配置: %w", action, err)
		}
		return svc, nil
	}
	if err := svc.InitRuntimeConfig(); err != nil {
		return nil, fmt.Errorf("%s失败: 读取运行时配置: %w", action, err)
	}
	return svc, nil
}
