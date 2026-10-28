package app

import (
	"fmt"

	"myblogx/conf"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/platform/cdc/river"
	"myblogx/internal/platform/health"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/platform/safego"
)

// 后台组件在健康检查里的名字。用常量而不是散落的字面量：
// 探测脚本、告警规则、文档都按这些名字对账，拼错一个字母就变成"永远没有这个组件"。
const (
	componentCDCRiver = "cdc_river"
	componentImageRef = "image_ref_river"
)

// WireRiver 组装并启动 MySQL->ES 同步 worker。
//
// # 为什么每一步都要登记健康状态
//
// 2026-10-29 的生产事故：全量转储因为 dump 客户端不兼容而失败（根因见 bad277c），
// canal 起不来、river 静默退出。当时进程照常监听端口、/health 照常 200，
// 唯一留下的痕迹是日志里一行「River 运行失败」——而没人盯日志。
// 表现就是"发文章成功，但首页永远不出现，且没有任何一处报错"。
//
// 所以这里把"没在跑"变成可查询的状态：未启用、构造失败、运行失败分别登记，
// /health/ready 汇总返回，探针能直接发现。
func WireRiver(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	if !infra.Config.River.Enabled {
		// 未启用也要登记：否则"CDC 没启用"和"CDC 挂了"在观测上完全无法区分。
		infra.Health.Set(componentCDCRiver, health.StateDisabled, nil)
		infra.Logger.Infof("配置中未启用 MySQL 同步任务")
		return nil
	}

	r, err := river.NewRiver(infra.Config.River, infra.Logger, infra.DB, infra.ESClient)
	if err != nil {
		// 构造失败（连不上 MySQL、拿不到 FULL 行镜像）意味着同步根本没开始。
		// 顺手登记成失败，避免"装配失败 → 进程继续跑 → 探针全绿"。
		infra.Health.Set(componentCDCRiver, health.StateFailed, err)
		return fmt.Errorf("创建 River 失败: %w", err)
	}
	r.SetLogOptions(writer.NewOptions(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	infra.Health.Set(componentCDCRiver, health.StateStarting, nil)

	// Run 是阻塞的：正常返回表示同步循环结束（停机），返回错误表示根本没起来。
	// "起来了没有"的分界线就在这里，比"日志里看起来没报错"可靠得多。
	safego.Go(func(recovered any, stack []byte) {
		infra.Health.Set(componentCDCRiver, health.StateFailed, fmt.Errorf("panic: %v", recovered))
		if infra.Logger != nil {
			infra.Logger.Errorf("River 运行 panic: %v\n%s", recovered, stack)
		}
	}, func() {
		if runErr := r.Run(); runErr != nil {
			infra.Health.Set(componentCDCRiver, health.StateFailed, runErr)
			if infra.Logger != nil {
				infra.Logger.Errorf("River 运行失败: %v", runErr)
			}
			return
		}
		infra.Health.Set(componentCDCRiver, health.StateStopped, nil)
	})

	// 走到这里说明 goroutine 已经起来、Run 还没返回。
	// 这个"运行中"是乐观的：若 Run 立刻失败，上面的回调会紧接着把它改成 failed
	// （Set 按状态变化去重，两次调用不会互相覆盖出错）。没有更精确的"启动成功"
	// 回调可用——canal 在 RunFrom 里同步做完位点解析与全量转储，中间不暴露钩子。
	infra.Health.Set(componentCDCRiver, health.StateRunning, nil)

	return nil
}

// WireImageRef 组装并启动图片引用监听 worker。
//
// 与 river 同构（也是 goroutine 里 Run、失败只写日志），挂掉的表现更隐蔽：
// 引用关系不再更新，删图时不会拦"这张图还被文章用着"，而是直接删掉正在用的图。
func WireImageRef(infra *Infra) error {
	if err := validateInfra(infra); err != nil {
		return err
	}

	if !infra.Config.ImageRefRiver.Enabled {
		infra.Health.Set(componentImageRef, health.StateDisabled, nil)
		infra.Logger.Infof("配置中未启用图片引用监听")
		return nil
	}

	r, err := imageref.NewRiver(infra.Config.ImageRefRiver, func() conf.QiNiu { return infra.RuntimeSite.GetRuntimeStorage() }, infra.Logger, infra.DB)
	if err != nil {
		infra.Health.Set(componentImageRef, health.StateFailed, err)
		return fmt.Errorf("创建 ImageRef River 失败: %w", err)
	}
	r.SetLogOptions(writer.NewOptions(infra.Config.Log, infra.Config.System, infra.Config.ClickHouse.Enabled, infra.Logger, infra.ClickHouse))

	infra.Health.Set(componentImageRef, health.StateStarting, nil)

	safego.Go(func(recovered any, stack []byte) {
		infra.Health.Set(componentImageRef, health.StateFailed, fmt.Errorf("panic: %v", recovered))
		if infra.Logger != nil {
			infra.Logger.Errorf("图片引用监听 panic: %v\n%s", recovered, stack)
		}
	}, func() {
		if runErr := r.Run(); runErr != nil {
			infra.Health.Set(componentImageRef, health.StateFailed, runErr)
			if infra.Logger != nil {
				infra.Logger.Errorf("图片引用监听运行失败: %v", runErr)
			}
			return
		}
		infra.Health.Set(componentImageRef, health.StateStopped, nil)
	})

	infra.Health.Set(componentImageRef, health.StateRunning, nil)

	return nil
}

// WireCron 见 app/cron_wiring.go：定时任务的编排在那里（刷盘机制、写入 SQL、
// 调度器已经分到各自的包里，装配层只负责把"哪个 key 刷到哪张表"接起来）。

// WireWorker 组装并启动所有后台 worker（river + image-ref + cron）。
func WireWorker(infra *Infra, lifecycle *Lifecycle) error {
	if err := WireRiver(infra); err != nil {
		return err
	}
	if err := WireImageRef(infra); err != nil {
		return err
	}
	return WireCron(infra, lifecycle)
}
