package app

import (
	"fmt"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/platform/cdc/river"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/safego"
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

	r, err := river.NewRiver(infra.Config.River, infra.Logger, infra.DB, infra.ESClient)
	if err != nil {
		return fmt.Errorf("创建 River 失败: %w", err)
	}
	r.SetLogOptions(writer.NewOptions(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	safego.Go(func(r any, stack []byte) {
		if infra.Logger != nil {
			infra.Logger.Errorf("River 运行 panic: %v\n%s", r, stack)
		}
	}, func() {
		if runErr := r.Run(); runErr != nil && infra.Logger != nil {
			infra.Logger.Errorf("River 运行失败: %v", runErr)
		}
	})
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

	r, err := imageref.NewRiver(infra.Config.ImageRefRiver, infra.Config.QiNiu, infra.Logger, infra.DB)
	if err != nil {
		return fmt.Errorf("创建 ImageRef River 失败: %w", err)
	}
	r.SetLogOptions(writer.NewOptions(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	safego.Go(func(r any, stack []byte) {
		if infra.Logger != nil {
			infra.Logger.Errorf("图片引用监听 panic: %v\n%s", r, stack)
		}
	}, func() {
		if runErr := r.Run(); runErr != nil && infra.Logger != nil {
			infra.Logger.Errorf("图片引用监听运行失败: %v", runErr)
		}
	})
	return nil
}

// WireCron 见 app/cron_wiring.go：定时任务的编排在那里（刷盘机制、写入 SQL、
// 调度器已经分到各自的包里，装配层只负责把"哪个 key 刷到哪张表"接起来）。

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
