// 数据库配置

package conf

import (
	"fmt"
)

type DB struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	Source   string `yaml:"source"` // 数据库的源
}

func (d *DB) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.DBName)
}

func (d *DB) SafeDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		d.User, "******", d.Host, d.Port, d.DBName)
}

func (d DB) Empty() bool {
	return d.User == "" && d.Password == "" && d.Host == "" && d.Port == 0 && d.DBName == ""
}

func (d DB) Addr() string {
	return fmt.Sprintf("%s:%d", d.Host, d.Port)
}
