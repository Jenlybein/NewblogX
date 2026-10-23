package core

import (
	"myblogx/conf"
	"myblogx/service/image_ref_river_service"
	"myblogx/service/log_service"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type ImageRefRiverDeps struct {
	ImageRefRiverConfig conf.ImageRefRiver
	QiNiuConfig         conf.QiNiu
	LogConfig           conf.Logrus
	System              conf.System
	ClickHouseEnabled   bool
	Logger              *logrus.Logger
	DB                  *gorm.DB
}

func InitImageRefRiver(deps ImageRefRiverDeps) {
	if !deps.ImageRefRiverConfig.Enabled {
		deps.Logger.Infof("配置中未启用图片引用监听")
		return
	}

	r, err := image_ref_river_service.NewRiver(deps.ImageRefRiverConfig, deps.QiNiuConfig, deps.Logger, deps.DB)
	if err != nil {
		deps.Logger.Fatal(err)
	}
	r.SetLogDeps(log_service.NewDeps(deps.LogConfig, deps.System, deps.ClickHouseEnabled, deps.Logger, nil))
	go func() {
		if err := r.Run(); err != nil {
			deps.Logger.Errorf("图片引用监听运行失败: %v", err)
		}
	}()
}
