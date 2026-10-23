// 配置初始化

package core

import (
	"fmt"
	"io"
	"os"

	"myblogx/conf"
	"myblogx/utils/envyaml"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func ReadCfg(settings *string) (c *conf.Config) {
	byteData, err := os.ReadFile(*settings)
	if err != nil {
		panic(err)
	}

	c = new(conf.Config)

	err = envyaml.Unmarshal(byteData, c)

	if err != nil {
		panic(fmt.Errorf("yaml 配置文件解析失败: %s", err))
	}

	fmt.Printf("读取配置文件 %s 成功\n", *settings)

	return c
}

func SetCfg(cfg *conf.Config, settings *string, logger *logrus.Logger) {
	byteData, err := yaml.Marshal(*cfg)
	if err != nil {
		logError(logger, "yaml 配置文件序列化失败: %s", err)
	}

	err = os.WriteFile(*settings, byteData, 0666)
	if err != nil {
		logError(logger, "yaml 配置文件写入失败: %s", err)
	}
}

func logError(logger *logrus.Logger, format string, args ...any) {
	if logger != nil {
		logger.Errorf(format, args...)
		return
	}
	_, _ = fmt.Fprintf(io.Discard, format, args...)
}
