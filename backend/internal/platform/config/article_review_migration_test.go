package config

import (
	"encoding/json"
	"testing"

	"myblogx/conf"
)

// 2026-11-28 / ADR-0037：这批迁移把三件事收进 -db，三条都必须能逐字验证幂等。
//
// 为什么逐字而不是逐键：迁移会重排 JSON 键（map 遍历顺序随机），如果不排序，
// "再跑一次结果不变"就只能靠语义断言 —— 那等于放弃了最便宜的那道检查。

const legacySiteArticleReviewPayload = `{"site_info":{"title":"x"},"article":{` +
	`"skip_examining":false,"submit_daily_max":15,"quality_auto_release":false,` +
	`"quality_gate":{"enabled":true,"pass_score":50,"gray_band_width":2},` +
	`"max_tags_per_article":5,"allow_non_admin_publish":true,"allow_non_admin_draft":true}}`

const legacyContentFilterMaskPayload = `{"words":[],"whitelist":[],` +
	`"whitelist_phrases":[],"retain_days":90,` +
	`"auto_dispose":{"block_action":"mask","watch_action":"mask"}}`

func TestPlanArticleReviewMigration_旧开关false时开启AI合规(t *testing.T) {
	payloads := map[string]string{
		string(SectionSite): legacySiteArticleReviewPayload,
	}
	sitePayload, _, plan, err := planArticleReviewMigration(payloads)
	if err != nil {
		t.Fatalf("计划失败: %v", err)
	}
	if !plan.siteChanged {
		t.Fatal("有 quality_auto_release 时 site 段必须被改写")
	}
	if plan.qualityAutoRelease == nil || *plan.qualityAutoRelease {
		t.Fatalf("应读到旧值 false，实际 %+v", plan.qualityAutoRelease)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sitePayload), &top); err != nil {
		t.Fatalf("产物不是合法 JSON: %v", err)
	}
	var article map[string]json.RawMessage
	if err := json.Unmarshal(top["article"], &article); err != nil {
		t.Fatalf("article 不是对象: %v", err)
	}
	if _, exists := article["quality_auto_release"]; exists {
		t.Fatal("旧键必须被物理删除（不留第二份事实源）")
	}
	// false ⇒ 保持 AI 合规判定开启（改造前的等价行为）。
	var aiEnabled bool
	if err := json.Unmarshal(article["ai_compliance_enabled"], &aiEnabled); err != nil || !aiEnabled {
		t.Fatalf("quality_auto_release=false 应写 ai_compliance_enabled=true，实际 %s", article["ai_compliance_enabled"])
	}
}

func TestPlanArticleReviewMigration_旧开关true时关闭AI合规(t *testing.T) {
	payloads := map[string]string{
		string(SectionSite): `{"article":{"quality_auto_release":true,"submit_daily_max":15}}`,
	}
	sitePayload, _, plan, err := planArticleReviewMigration(payloads)
	if err != nil {
		t.Fatalf("计划失败: %v", err)
	}
	if !plan.siteChanged {
		t.Fatal("应被改写")
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal([]byte(sitePayload), &top)
	var article map[string]json.RawMessage
	_ = json.Unmarshal(top["article"], &article)
	var aiEnabled bool
	_ = json.Unmarshal(article["ai_compliance_enabled"], &aiEnabled)
	if aiEnabled {
		t.Fatal("quality_auto_release=true（质量过线就不等 AI）应写 ai_compliance_enabled=false")
	}
	// 缺键的两个开关也要补上：缺键时结构体零值是"关"，会把线上行为翻转。
	var scoreEnabled bool
	if err := json.Unmarshal(article["score_enabled"], &scoreEnabled); err != nil || !scoreEnabled {
		t.Fatalf("score_enabled 缺键时应补 true，实际 %s", article["score_enabled"])
	}
	var action string
	if err := json.Unmarshal(article["ai_failure_action"], &action); err != nil || action != "manual" {
		t.Fatalf("ai_failure_action 缺键时应补 manual，实际 %s", article["ai_failure_action"])
	}
}

func TestPlanArticleReviewMigration_已迁移后不需要再改(t *testing.T) {
	payloads := map[string]string{
		string(SectionSite): `{"article":{"skip_examining":false,"submit_daily_max":15,` +
			`"score_enabled":true,"ai_compliance_enabled":true,"ai_failure_action":"manual",` +
			`"quality_gate":{"enabled":true,"pass_score":50,"gray_band_width":2}}}`,
		string(SectionContentFilter): `{"auto_dispose":{"block_action":"release","watch_action":"release"}}`,
	}
	_, _, plan, err := planArticleReviewMigration(payloads)
	if err != nil {
		t.Fatalf("计划失败: %v", err)
	}
	if plan.changed() {
		t.Fatalf("已经迁移过的库不该再报改动（幂等），实际 site=%v filter=%v",
			plan.siteChanged, plan.filterChanged)
	}
}

func TestPlanArticleReviewMigration_旧mask读成release(t *testing.T) {
	payloads := map[string]string{
		string(SectionContentFilter): legacyContentFilterMaskPayload,
	}
	_, filterPayload, plan, err := planArticleReviewMigration(payloads)
	if err != nil {
		t.Fatalf("计划失败: %v", err)
	}
	if !plan.filterChanged {
		t.Fatal("有 mask 时 content_filter 段必须被改写")
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal([]byte(filterPayload), &top)
	var dispose map[string]json.RawMessage
	_ = json.Unmarshal(top["auto_dispose"], &dispose)
	for _, key := range []string{"block_action", "watch_action"} {
		var action string
		_ = json.Unmarshal(dispose[key], &action)
		if action != "release" {
			t.Fatalf("%s 的旧值 mask 应写成 release，实际 %q", key, action)
		}
	}
	// 词表等其它键必须原样保留（迁移只碰 auto_dispose）。
	var retainDays int
	_ = json.Unmarshal(top["retain_days"], &retainDays)
	if retainDays != 90 {
		t.Fatalf("retain_days 不该被迁移改动，实际 %d", retainDays)
	}
}

func TestPlanArticleReviewMigration_两次执行产出同一份字节(t *testing.T) {
	payloads := map[string]string{
		string(SectionSite):          legacySiteArticleReviewPayload,
		string(SectionContentFilter): legacyContentFilterMaskPayload,
	}
	site1, filter1, _, err := planArticleReviewMigration(payloads)
	if err != nil {
		t.Fatalf("第一次计划失败: %v", err)
	}

	// 把第一次的结果当成新库再跑一次：必须既不报改动、也不再产生新字节。
	second := map[string]string{
		string(SectionSite):          site1,
		string(SectionContentFilter): filter1,
	}
	site2, filter2, plan2, err := planArticleReviewMigration(second)
	if err != nil {
		t.Fatalf("第二次计划失败: %v", err)
	}
	if plan2.changed() {
		t.Fatalf("第二次不该报改动，实际 site=%v filter=%v", plan2.siteChanged, plan2.filterChanged)
	}
	if site2 != "" || filter2 != "" {
		t.Fatalf("第二次不该产出新 payload，实际 site=%d 字节 filter=%d 字节", len(site2), len(filter2))
	}
}

func TestPlanArticleReviewMigration_没有相关段时无事可做(t *testing.T) {
	_, _, plan, err := planArticleReviewMigration(map[string]string{})
	if err != nil {
		t.Fatalf("空库不该报错: %v", err)
	}
	if plan.changed() {
		t.Fatal("空库不该报改动")
	}
}

// TestMigrateArticleReview_幂等与dryRun 用内存库跑一遍真实写库路径。
func TestMigrateArticleReview_幂等与dryRun(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})

	if err := upsertPayload(db, SectionSite, legacySiteArticleReviewPayload); err != nil {
		t.Fatalf("预置 site 载荷失败: %v", err)
	}
	if err := upsertPayload(db, SectionContentFilter, legacyContentFilterMaskPayload); err != nil {
		t.Fatalf("预置 content_filter 载荷失败: %v", err)
	}

	// dry-run：必须发现改动，但一个字节都不写。
	result, err := svc.MigrateArticleReview(false)
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if !result.Changed() {
		t.Fatal("库里还有旧键时，报告必须说发现了要改的东西")
	}
	if result.Applied {
		t.Fatal("dry-run 不得标记为已写库")
	}
	if payload := readPayload(t, db, SectionSite); payload != legacySiteArticleReviewPayload {
		t.Fatalf("dry-run 不得改动库里的 payload，实际 %s", payload)
	}

	// apply：写库并报告已写。
	applied, err := svc.MigrateArticleReview(true)
	if err != nil {
		t.Fatalf("apply 失败: %v", err)
	}
	if !applied.Applied || !applied.Changed() {
		t.Fatalf("apply 必须标记为已写库且有改动，实际 %+v", applied)
	}

	// 第二次 apply：幂等（不报改动、不写库）。
	again, err := svc.MigrateArticleReview(true)
	if err != nil {
		t.Fatalf("第二次 apply 失败: %v", err)
	}
	if again.Changed() || again.Applied {
		t.Fatalf("第二次 apply 必须幂等，实际 %+v", again)
	}
}
