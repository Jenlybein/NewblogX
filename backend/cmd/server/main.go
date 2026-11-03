// 主程序入口

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"myblogx/app"
	"myblogx/buildinfo"
	"myblogx/cmd/cli"
	"myblogx/core"
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

	flag := cli.Parse()
	config := core.ReadCfg(&flag.File)

	infra, err := app.InitInfra(config, flag.File, buildinfo.Version)
	if err != nil {
		panic(err)
	}

	runResult, err := cli.Run(flag, cli.Options{
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
	startOutboxRelay := func(infra *app.Infra, role string, lifecycle *app.Lifecycle) {
		app.WireOutboxRelay(infra, outboxWiringOptionsForRole(role))
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
		startOutboxRelay(infra, role, lifecycle)
		lifecycle.WaitForShutdown()
		return
	case "river":
		if err := app.WireRiver(infra); err != nil {
			infra.Logger.Fatalf("River 组装失败: %v", err)
		}
		startOutboxRelay(infra, role, lifecycle)
		lifecycle.WaitForShutdown()
		return
	case "image-ref":
		if err := app.WireImageRef(infra); err != nil {
			infra.Logger.Fatalf("ImageRef 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "cron":
		if err := app.WireCron(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("Cron 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "worker":
		if err := app.WireWorker(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("Worker 组装失败: %v", err)
		}
		lifecycle.WaitForShutdown()
		return
	case "all":
		if err := app.WireWorker(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("Worker 组装失败: %v", err)
		}
		if err := app.WireHTTP(infra, lifecycle); err != nil {
			infra.Logger.Fatalf("HTTP 组装失败: %v", err)
		}
		// all 角色同时承担 HTTP 与后台任务，也必须启动 outbox 中继：
		// 领域事件的副作用（通知、评分、私信 WS 通知）都靠它投递，
		// 少了它这些副作用会被静默丢弃（事件只在表里 pending）。
		// 注意顺序：必须在 WireHTTP 之后，私信的 WS 通知订阅者才拿得到连接池。
		// 但"是否需要跳过私信事件"由 outboxWiringOptionsForRole 显式给出，
		// 不再依赖这个顺序来推导。
		startOutboxRelay(infra, role, lifecycle)
		lifecycle.WaitForShutdown()
		return
	default:
		infra.Logger.Fatalf("未知 role 参数: %s，可选值: api|river|image-ref|cron|worker|all", role)
	}
}

// outboxWiringOptionsForRole 是"哪个进程角色能投私信通知"的唯一映射。
//
// 显式列出而不是从"WireChat 跑没跑过"反推：api / all 持有 WS 连接池，
// 可以领取 chat.message.sent / chat.message.read；river 没有连接池，必须跳过。
// 调用顺序因此不再影响行为。守卫测试见 outbox_role_test.go。
func outboxWiringOptionsForRole(role string) app.OutboxWiringOptions {
	if role == "river" {
		return app.OutboxWiringOptions{DeliverChatPush: false}
	}
	// api / all（以及未来任何会 WireHTTP 的角色）都持有连接池。
	return app.OutboxWiringOptions{DeliverChatPush: true}
}
