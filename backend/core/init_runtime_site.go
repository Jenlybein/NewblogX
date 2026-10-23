package core

import (
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func InitRuntimeSite(baseConfig *conf.Config, logger *logrus.Logger, db *gorm.DB, configFile string) (*site_service.RuntimeConfigService, error) {
	runtimeService := site_service.NewRuntimeConfigService(baseConfig.Site, baseConfig.AI, logger, db, configFile)
	if err := runtimeService.InitRuntimeConfig(); err != nil {
		return nil, err
	}
	return runtimeService, nil
}
