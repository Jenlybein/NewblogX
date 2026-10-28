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
//   - 进程级段（log）来自 settings.yaml —— 日志写到哪儿、服务名是什么，
//     属于部署决定，不是站点配置。
func InitRuntimeSite(baseConfig *conf.Config, logger *logrus.Logger, db *gorm.DB, configFile string) (*sitecfg.RuntimeConfigService, error) {
	defaults, err := sitecfg.LoadDefaults(configFile)
	if err != nil {
		// 默认值文件读不到不该拦住启动：数据库里已有的配置仍然可用。
		// 但要退到 settings.yaml 的基线值，而不是全零 —— 日志级别退成空字符串
		// 会让 logrus 退回 info，看起来没事，但库里的值也跟着变成空的。
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

	// 后台改日志级别要立刻生效：把"下发到 logger"的能力交回去。
	if logger != nil {
		runtimeService.SetLogApplier(func(log conf.Logrus) {
			ApplyLogLevel(logger, log.Level)
			logger.Infof("日志级别已更新为 %s", log.Level)
		})
	}
	// 启动时也按库里的值对齐一次：库里可能存着上次在后台改过的级别，
	// 而 logger 此时还是 settings.yaml 的基线。
	ApplyLogLevel(logger, runtimeService.GetRuntimeLog().Level)

	return runtimeService, nil
}

// baseDefaults 用 settings.yaml 的基线值组装默认值。
//
// 这是 site_default_settings.yaml 读不到时的兜底路径。settings.yaml 现在只剩
// 进程级配置，业务段（含 mail / storage / oauth / upload）在这里没有基线，
// 所以兜底值全是零：对象存储关闭、上传白名单为空（等于拒绝所有格式）。
// 这是有意的 fail closed —— 默认值文件是随镜像走的，读不到说明部署出了问题，
// 此时"能传任意格式"比"传不了"危险得多，而且后台随时能自己补。
func baseDefaults(baseConfig *conf.Config) conf.RuntimeDefaults {
	if baseConfig == nil {
		return conf.RuntimeDefaults{}
	}
	return conf.RuntimeDefaults{Log: baseConfig.Log}
}
