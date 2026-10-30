// 系统配置

package conf

import (
	"fmt"
)

// 环境标识的取值口径：BLOGX_APP_ENV 写 production 表示生产环境。
const EnvProduction = "production"

// EnvProductionLegacy 是历史写法 "prod"，不再允许新配置使用。
//
// 保留它只为了兼容升级前写死 prod 的 .env：若代码只认 production，
// 那些部署升级后会静默失去 refresh Cookie 的 Secure。启动时会告警提示迁移。
const EnvProductionLegacy = "prod"

type System struct {
	ServerID uint32 `yaml:"server_id"` // 雪花 ID 机器号，生产环境需保证每个实例唯一
	IP       string `yaml:"ip"`
	Port     int    `yaml:"port"`
	Env      string `yaml:"env"`
	GinMode  string `yaml:"gin_mode"`
}

func (s *System) Addr() string {
	return fmt.Sprintf("%s:%d", s.IP, s.Port)
}

// IsProduction 报告是否按生产环境运行（refresh Cookie 因此加 Secure）。
func (s System) IsProduction() bool {
	return s.Env == EnvProduction || s.Env == EnvProductionLegacy
}
