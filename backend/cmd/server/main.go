// 主程序入口

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"myblogx/app"
	"myblogx/buildinfo"
	"myblogx/core"
	"myblogx/flags"
)

// wantsVersion 判断命令行是否只想要版本号。
//
// 必须在读取配置之前处理：否则运维只想确认"线上跑的是哪个提交"，
// 却会先撞上"缺少 N 个环境变量"的 panic。
func wantsVersion(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-version" || a == "-v" {
			return true
		}
	}
	return false
}

// @title BlogX API
// @version 1.0
// @description BlogX 后端 API 契约（由 swag 从代码注解生成，勿手工编辑产物）。
// @BasePath /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	if wantsVersion(os.Args[1:]) {
		fmt.Println(buildinfo.String())
		return
	}

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

	// startOutboxRelay 启动 outbox 中继并登记停机钩子。
	//
	// 独立于 river：即使没启用 ES 同步，领域事件也必须投递。
	// 停机时先停中继，避免在途投递被强行打断后留下 processing 状态的行
	// （真留下了也没关系，租约到期后会被下一轮补偿扫描捞回）。
	startOutboxRelay := func(infra *app.Infra, lifecycle *app.Lifecycle) {
		app.WireOutboxRelay(infra)
		lifecycle.OnStop("outbox_relay", func(ctx context.Context) error {
			app.CloseOutboxRelay()
			return nil
		})
	}

	role := strings.ToLower(strings.TrimSpace(flag.Role))
	lifecycle := app.NewLifecycle(infra.Logger)
	app.RegisterInfraCleanup(lifecycle, infra)

	switch role {
	case "api":
		if err := app.WireHTTP(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("HTTP 组装失败: %v", err)
		}
		// 领域事件在这里产生，也就在这里投递；river 角色下同样会启动一份，
		// 两边靠"原子领取"保证同一条事件只会被一个进程处理。
		startOutboxRelay(infra, lifecycle)
		lifecycle.WaitForShutdown()
		return
	case "river":
		if err := app.WireRiver(infra); err != nil {
			infra.Logger.Fatalf("River 组装失败: %v", err)
		}
		startOutboxRelay(infra, lifecycle)
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
		if err := app.WireHTTP(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("HTTP 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	default:
		infra.Logger.Fatalf("未知 role 参数: %s，可选值: api|river|image-ref|cron|worker|all", role)
	}
}
