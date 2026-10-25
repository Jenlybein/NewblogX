package core

import (
	"myblogx/conf"
	sitecfg "myblogx/internal/platform/config"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func InitRuntimeSite(baseConfig *conf.Config, logger *logrus.Logger, db *gorm.DB, configFile string) (*sitecfg.RuntimeConfigService, error) {
	runtimeService := sitecfg.NewRuntimeConfigService(baseConfig.Site, baseConfig.AI, logger, db, configFile)
	if err := runtimeService.InitRuntimeConfig(); err != nil {
		return nil, err
	}
	return runtimeService, nil
}
