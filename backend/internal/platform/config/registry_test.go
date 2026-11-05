package config

import (
	"strings"
	"testing"

	"myblogx/conf"
)

// 段注册表是九段唯一事实：任何未登记的段在写入后必须失败，而不是
// "警告后缓存不更新"。这条用例只验 UpdateXxx 外部可见的返回值与缓存状态，
// 不读源码里有没有 Warnf / switch / case。
func TestUnregisteredSectionFailsInsteadOfSilentCacheMiss(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{
		Mail: conf.Mail{SecretID: "registered-baseline"},
	})

	// 构造一段注册表之外的值：类型本身是九段之一，段名却未登记。
	// 从公共入口看，它的效果和"新增了第十段却忘了登记"完全一样。
	unregistered := conf.Mail{SecretID: "unregistered-value"}
	err := updateSection(svc, SectionName(unregisteredSectionName), unregistered)
	if err == nil {
		t.Fatal("未登记的段必须返回错误，不能警告后静默放行")
	}
	if !strings.Contains(err.Error(), unregisteredSectionName) {
		t.Fatalf("错误应点明漏登记的段名 %q，实际: %v", unregisteredSectionName, err)
	}

	// 失败不能留下"库里写进去了、缓存却没更新"这种半状态。
	var count int64
	if err := db.Model(&runtimeConfigRow{}).Where("name = ?", unregisteredSectionName).Count(&count).Error; err != nil {
		t.Fatalf("查询未登记段失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("未登记段失败后不应留下库行，实际 %d 行", count)
	}
	if got := svc.GetRuntimeMail().SecretID; got != "registered-baseline" {
		t.Fatalf("失败不能改动已登记的缓存值，实际 %q", got)
	}
}

// 九段全部登记、且每个登记项的解码结果都能写回缓存（漏登记或类型写歪即红）。
// 期望值来自票面的九段清单，不从被测试的注册表反推，避免自证。
func TestSectionRegistryRegistersAllNineSections(t *testing.T) {
	want := []string{
		"site", "ai", "mail", "storage", "oauth", "log", "upload", "deco", "image_gc",
	}
	svc, _ := newTestService(t, conf.RuntimeDefaults{})
	for _, name := range want {
		entry, ok := LookupSection(name)
		if !ok {
			t.Errorf("段 %q 未登记", name)
			continue
		}
		decoded := entry.Decode(svc, map[string]string{})
		if !entry.Apply(&RuntimeConfig{}, decoded) {
			t.Errorf("段 %q 的解码结果无法写回缓存", name)
		}
	}
	if got := len(RegisteredSections()); got != len(want) {
		t.Errorf("注册表应恰好登记 %d 段，实际 %d", len(want), got)
	}
}

// unregisteredSectionName 故意不在 Registry 里；用段名（而不是 Go 类型）当键，
// 因为漏登记表现为"名字没有 handler"，不是"类型没人认识"。
const unregisteredSectionName = "unregistered_section"
