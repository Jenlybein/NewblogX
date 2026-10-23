// 系统配置

package conf

import (
	"fmt"
)

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
