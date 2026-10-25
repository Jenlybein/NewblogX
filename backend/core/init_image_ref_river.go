package core

import (
	"myblogx/conf"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/safego"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type ImageRefRiverOptions struct {
	ImageRefRiverConfig conf.ImageRefRiver
	QiNiuConfig         conf.QiNiu
	LogConfig           conf.Logrus
	System              conf.System
	ClickHouseEnabled   bool
	Logger              *logrus.Logger
	DB                  *gorm.DB
}

func InitImageRefRiver(deps ImageRefRiverOptions) {
	if !deps.ImageRefRiverConfig.Enabled {
		deps.Logger.Infof("配置中未启用图片引用监听")
		return
	}

	r, err := imageref.NewRiver(deps.ImageRefRiverConfig, deps.QiNiuConfig, deps.Logger, deps.DB)
	if err != nil {
		deps.Logger.Fatal(err)
	}
	r.SetLogOptions(writer.NewOptions(deps.LogConfig, deps.System, deps.ClickHouseEnabled, deps.Logger, nil))
	safego.Go(func(recovered any, stack []byte) {
		deps.Logger.Errorf("图片引用监听 panic: %v\n%s", recovered, stack)
	}, func() {
		if err := r.Run(); err != nil {
			deps.Logger.Errorf("图片引用监听运行失败: %v", err)
		}
	})
}
