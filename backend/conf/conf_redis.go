package conf

import "strconv"

type Redis struct {
	Addr     string `yaml:"addr"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

func (r *Redis) GetAddr() string {
	return r.Addr + ":" + strconv.Itoa(r.Port)
}
