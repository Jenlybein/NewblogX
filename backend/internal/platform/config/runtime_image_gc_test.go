package config

import (
	"strings"
	"testing"

	"myblogx/conf"
)

// image_gc 是 issue #139 新增的运行时配置段。这组用例守"段注册漏了一处"：
// 漏掉段名常量 / 结构体字段 / 全量解码 / 缺失补种 / 缓存回写 / 空缓存默认
// 里任何一处，症状都是"改了没反应"或"读到零值"，而编译器一个字都不会说。

func TestImageGC段_缺省补行且更新立刻生效(t *testing.T) {
	defaults := conf.RuntimeDefaults{
		ImageGC: conf.ImageGC{Enabled: true, GraceDays: 14, BatchLimit: 50},
	}
	svc, db := newTestService(t, defaults)

	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	if got := svc.GetRuntimeImageGC(); !got.Enabled || got.GraceDays != 14 || got.BatchLimit != 50 {
		t.Fatalf("新段的默认值没进缓存: %+v", got)
	}

	// 更新必须落库 + 立刻进缓存：冷静期是运行时配置，改完不必重启。
	// 回收开关同理 —— 异常时改一行配置就要能立刻停手。
	if err := svc.UpdateImageGC(conf.ImageGC{Enabled: false, GraceDays: 7, BatchLimit: 20}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if got := svc.GetRuntimeImageGC(); got.Enabled || got.GraceDays != 7 || got.BatchLimit != 20 {
		t.Fatalf("更新后缓存没刷新: %+v", got)
	}
	payload := readPayload(t, db, SectionImageGC)
	if !strings.Contains(payload, "grace_days") || !strings.Contains(payload, "batch_limit") || !strings.Contains(payload, "enabled") {
		t.Fatalf("落库的 payload 不含新段的字段: %s", payload)
	}
}
