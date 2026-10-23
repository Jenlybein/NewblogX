// 主程序入口

package main

import (
	"myblogx/app"
	"myblogx/buildinfo"
	"myblogx/core"
	"myblogx/flags"
	"strings"
)

func main() {
	flag := flags.Parse()
	config := core.ReadCfg(&flag.File)

	infra, err := app.InitInfra(config, flag.File, buildinfo.Version)
	if err != nil {
		panic(err)
	}

	runResult, err := flags.Run(flag, flags.Deps{
		RiverConfig: infra.Config.River,
		Logger:      infra.Logger,
		DB:          infra.DB,
		ESClient:    infra.ESClient,
		ESIndex:     infra.Config.ES.Index,
	})
	if err != nil {
		panic(err)
	}
	if !runResult.ContinueStartup {
		return
	}

	if err = app.InitRuntimeServices(infra); err != nil {
		panic(err)
	}

	role := strings.ToLower(strings.TrimSpace(flag.Role))
	lifecycle := app.NewLifecycle(infra.Logger)

	switch role {
	case "api":
		if err := app.WireHTTP(infra); err != nil {
			infra.Logger.Fatalf("HTTP 组装失败: %v", err)
		}
		return
	case "river":
		if err := app.WireRiver(infra); err != nil {
			infra.Logger.Fatalf("River 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "image-ref":
		if err := app.WireImageRef(infra); err != nil {
			infra.Logger.Fatalf("ImageRef 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "cron":
		if err := app.WireCron(infra); err != nil {
			infra.Logger.Fatalf("Cron 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "worker":
		if err := app.WireWorker(infra); err != nil {
			infra.Logger.Fatalf("Worker 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "all":
		if err := app.WireWorker(infra); err != nil {
			infra.Logger.Fatalf("Worker 组装失败: %v", err)
		}
		if err := app.WireHTTP(infra); err != nil {
			infra.Logger.Fatalf("HTTP 组装失败: %v", err)
		}
		return
	default:
		infra.Logger.Fatalf("未知 role 参数: %s，可选值: api|river|image-ref|cron|worker|all", role)
	}
}
