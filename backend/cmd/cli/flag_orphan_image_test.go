package cli

import (
	"path/filepath"
	"testing"

	"myblogx/conf"
	"myblogx/internal/platform/config"
)

// TestImageGCDefaultsComeFromSiteSettings 钉住"冷星期与单次上限来自运行时配置"。
//
// #139 的 AC 明确要求这两个量是运行时配置而不是写死的常量。这里直接读仓库
// 里那份 site_default_settings.yaml，确认 image_gc 段真的被解析出来。
// 以后有人挪动/改名这个段，这条用例会红。
func TestImageGCDefaultsComeFromSiteSettings(t *testing.T) {
	// cmd/cli 相对仓库根在 backend/cmd/cli。
	configFile := filepath.Join("..", "..", "config", "settings.yaml")

	defaults, err := config.LoadDefaults(configFile)
	if err != nil {
		t.Fatalf("读取运行时配置默认值失败: %v", err)
	}

	if defaults.ImageGC.CooldownDays != defaultOrphanCooldownDays {
		t.Fatalf("冷静期默认值应为 %d，实际 %d",
			defaultOrphanCooldownDays, defaults.ImageGC.CooldownDays)
	}
	if defaults.ImageGC.BatchLimit != defaultOrphanLimit {
		t.Fatalf("单次上限默认值应为 %d，实际 %d",
			defaultOrphanLimit, defaults.ImageGC.BatchLimit)
	}
}

// TestOrphanListPrefersConfigThenFlag 钉住取值优先级：配置 > flag 覆盖 > 兜底。
func TestOrphanListPrefersConfigThenFlag(t *testing.T) {
	configured := conf.ImageGC{CooldownDays: 21, BatchLimit: 50}
	unset := conf.ImageGC{}

	// 只有配置值时，用配置值。
	if got := resolveCooldownDays(configured, 0); got != 21 {
		t.Fatalf("应取配置里的冷静期 21，实际 %d", got)
	}
	// flag 覆盖优先。
	if got := resolveCooldownDays(configured, 3); got != 3 {
		t.Fatalf("flag 覆盖应生效（3），实际 %d", got)
	}
	// 配置缺省时退回兜底默认值。
	if got := resolveCooldownDays(unset, 0); got != defaultOrphanCooldownDays {
		t.Fatalf("配置缺省时应退回兜底 %d，实际 %d", defaultOrphanCooldownDays, got)
	}

	if got := resolveBatchLimit(configured, 0); got != 50 {
		t.Fatalf("应取配置里的上限 50，实际 %d", got)
	}
	if got := resolveBatchLimit(configured, 5); got != 5 {
		t.Fatalf("flag 覆盖应生效（5），实际 %d", got)
	}
	if got := resolveBatchLimit(unset, 0); got != defaultOrphanLimit {
		t.Fatalf("配置缺省时应退回兜底 %d，实际 %d", defaultOrphanLimit, got)
	}
}
