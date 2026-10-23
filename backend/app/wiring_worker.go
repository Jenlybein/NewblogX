package app

import (
	"fmt"
	"myblogx/service/cron_service"
	"myblogx/service/image_ref_river_service"
	"myblogx/service/log_service"
	"myblogx/service/river_service"
)

// WireRiver 组装并启动 MySQL->ES 同步 worker。
func WireRiver(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	if !infra.Config.River.Enabled {
		infra.Logger.Infof("配置中未启用 MySQL 同步任务")
		return nil
	}

	r, err := river_service.NewRiver(infra.Config.River, infra.Logger, infra.DB, infra.ESClient)
	if err != nil {
		return fmt.Errorf("创建 River 失败: %w", err)
	}
	r.SetLogDeps(log_service.NewDeps(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	go func() {
		if runErr := r.Run(); runErr != nil && infra.Logger != nil {
			infra.Logger.Errorf("River 运行失败: %v", runErr)
		}
	}()
	return nil
}

// WireImageRef 组装并启动图片引用监听 worker。
func WireImageRef(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	if !infra.Config.ImageRefRiver.Enabled {
		infra.Logger.Infof("配置中未启用图片引用监听")
		return nil
	}

	r, err := image_ref_river_service.NewRiver(infra.Config.ImageRefRiver, infra.Config.QiNiu, infra.Logger, infra.DB)
	if err != nil {
		return fmt.Errorf("创建 ImageRef River 失败: %w", err)
	}
	r.SetLogDeps(log_service.NewDeps(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	go func() {
		if runErr := r.Run(); runErr != nil && infra.Logger != nil {
			infra.Logger.Errorf("图片引用监听运行失败: %v", runErr)
		}
	}()
	return nil
}

// WireCron 组装并启动定时任务 worker。
func WireCron(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	cron_service.NewSchedulerRaw(infra.DB, infra.Redis, infra.Logger, infra.Config.Log).Start()
	return nil
}

// WireWorker 组装并启动所有后台 worker（river + image-ref + cron）。
func WireWorker(infra *Infra) error {
	if err := WireRiver(infra); err != nil {
		return err
	}
	if err := WireImageRef(infra); err != nil {
		return err
	}
	return WireCron(infra)
}
