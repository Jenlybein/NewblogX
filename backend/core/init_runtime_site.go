package core

import (
	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// InitRuntimeSite 加载运行时配置服务。
//
// 默认值有两个来源，按语义分工：
//   - 业务段（site/ai/mail/storage/oauth/upload）来自 site_default_settings.yaml，
//     因为它们本来就是"站点配置"；
//   - 进程级段（storage/oauth/upload/log）在 settings.yaml 里也有基线值，
//     文件读不到时用它们兜底。
func InitRuntimeSite(baseConfig *conf.Config, logger *logrus.Logger, db *gorm.DB, configFile string) (*sitecfg.RuntimeConfigService, error) {
	defaults, err := sitecfg.LoadDefaults(configFile)
	if err != nil {
		// 默认值文件读不到不该拦住启动：数据库里已有的配置仍然可用。
		// 但要退到 settings.yaml 的基线值，而不是全零 —— 退成全零会让
		// 上传白名单变成空数组（任何图片都传不了）、日志级别变成空字符串，
		// 而库里若还没有对应段，这些零值会被当成"默认值"写进去。
		if logger != nil {
			logger.Warnf("读取 site_default_settings.yaml 失败，回退到 settings.yaml 的基线值: %v", err)
		}
		defaults = baseDefaults(baseConfig)
	}
	// 日志始终以 settings.yaml 为准：目录、服务名、输出格式属于进程级配置，
	// 后台只允许覆盖级别、请求日志开关、查询限额、保留天数这些子集。
	defaults.Log = baseConfig.Log

	runtimeService := sitecfg.NewRuntimeConfigService(defaults, logger, db)
	if err := runtimeService.InitRuntimeConfig(); err != nil {
		return nil, err
	}
	return runtimeService, nil
}

// baseDefaults 用 settings.yaml 的基线值组装默认值。
//
// 这是 site_default_settings.yaml 读不到时的兜底路径。Mail 段没有基线：
// 历史上 settings.yaml 里的 email 段是 SMTP 结构，与腾讯云 SES 字段不同名，
// 不能直接搬过来。
func baseDefaults(baseConfig *conf.Config) conf.RuntimeDefaults {
	if baseConfig == nil {
		return conf.RuntimeDefaults{}
	}
	return conf.RuntimeDefaults{
		Site:    baseConfig.Site,
		AI:      baseConfig.AI,
		Storage: baseConfig.QiNiu,
		OAuth:   baseConfig.QQ,
		Log:     baseConfig.Log,
		Upload:  baseConfig.Upload,
	}
}
