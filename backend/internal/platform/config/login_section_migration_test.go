package config

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"myblogx/conf"
	siteconf "myblogx/conf/site"

	"gopkg.in/yaml.v3"
)

// site.login 按通道拆分（票 481）之后的一次性搬迁：`-migrate-runtime-config`。
//
// 这组用例要守住四件事：
//  1. 搬迁表覆盖全部十七个键（漏一个 = 升级时那个值静默丢失）；
//  2. site.login 里真的不再有这些键（用户明确要求不留兼容层）；
//  3. 默认只报告不写库，-apply 才写；目标段已有该键时保留目标值；
//  4. 幂等：第二次执行报告"无需迁移"且一个字节都不改。

// TestLoginSectionMigrationsCoverAllMovedFields 钉住搬迁表的内容。
//
// 期望值照票面清单写死、不从 loginSectionMigrations 反推：这条用例的作用正是
// 发现"表里少了一行"，反推的话它永远绿。
func TestLoginSectionMigrationsCoverAllMovedFields(t *testing.T) {
	want := map[string]SectionName{
		"email_login":              SectionMail,
		"email_register":           SectionMail,
		"email_code_timeout":       SectionMail,
		"email_send_window_second": SectionMail,
		"email_send_per_email_max": SectionMail,
		"email_send_per_ip_max":    SectionMail,

		"phone_login":            SectionSMS,
		"sms_code_timeout":       SectionSMS,
		"sms_send_window_second": SectionSMS,
		"sms_send_per_phone_max": SectionSMS,
		"sms_send_per_ip_max":    SectionSMS,

		"qq_login": SectionOAuth,

		"captcha":              SectionCaptcha,
		"captcha_login":        SectionCaptcha,
		"captcha_email_verify": SectionCaptcha,
		"captcha_phone_verify": SectionCaptcha,
		"captcha_register":     SectionCaptcha,
	}

	got := make(map[string]SectionName, len(loginSectionMigrations))
	for _, migration := range loginSectionMigrations {
		if prev, dup := got[migration.Key]; dup {
			t.Fatalf("搬迁表里 %q 出现了两次（前一次指向 %s）", migration.Key, prev)
		}
		got[migration.Key] = migration.Target
	}
	for key, target := range want {
		if got[key] != target {
			t.Errorf("键 %q 应搬到 %s，实际 %q", key, target, got[key])
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("搬迁表里多出未登记预期结果的键 %q（要么补进票面清单，要么删掉）", key)
		}
	}
}

// TestSiteLoginNoLongerCarriesMovedFields 钉住"结构体里已经删干净"。
//
// 迁移之所以必须存在，正是因为读取路径不再认识这些键；如果哪天有人把它们
// 加回 site.Login（"为了兼容"），这条用例会立刻红。
func TestSiteLoginNoLongerCarriesMovedFields(t *testing.T) {
	raw, err := jsonMarshalSite(t)
	if err != nil {
		t.Fatalf("序列化 site 段失败: %v", err)
	}
	login, ok := raw["login"]
	if !ok {
		t.Fatal("site 段必须仍有 login 子块")
	}
	keys := sortedKeys(login)
	want := []string{"login_fail_ip_max", "login_fail_user_max", "login_fail_window_minute", "username_pwd_login"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("site.login 只应剩 %v，实际 %v（被迁走的字段不得留在结构体里）", want, keys)
	}
}

// TestDefaultSettingsYaml_MovedDefaultsLiveInTheirOwnSections 钉住默认值搬到了新家。
//
// 默认值文件是"库里没有这段时的唯一来源"。字段搬了段却没搬默认值，新部署拿到的
// 就是零值：邮箱登录、短信登录、验证码闸门会全部变成 false，与升级前"看起来
// 一样但实际全关"。所以这里逐项核对**值**，而不只是"键存在"。
func TestDefaultSettingsYaml_MovedDefaultsLiveInTheirOwnSections(t *testing.T) {
	raw, err := os.ReadFile("../../../config/site_default_settings.yaml")
	if err != nil {
		t.Fatalf("读取默认值文件失败: %v", err)
	}
	var file struct {
		Site struct {
			Login map[string]any `yaml:"login"`
		} `yaml:"site"`
		Mail    map[string]any `yaml:"mail"`
		SMS     map[string]any `yaml:"sms"`
		OAuth   map[string]any `yaml:"oauth"`
		Captcha map[string]any `yaml:"captcha"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("解析默认值文件失败: %v", err)
	}

	if got, want := strings.Join(sortedKeys(file.Site.Login), ","),
		"login_fail_ip_max,login_fail_user_max,login_fail_window_minute,username_pwd_login"; got != want {
		t.Fatalf("site.login 的默认值只应剩 %s，实际 %s", want, got)
	}

	// 期望值取迁移前 site.login 里的实际值，逐个搬过来的就是这些。
	checks := []struct {
		section string
		values  map[string]any
		data    map[string]any
	}{
		{"mail", map[string]any{
			"email_login":              false,
			"email_register":           false,
			"email_code_timeout":       5,
			"email_send_window_second": 60,
			"email_send_per_email_max": 1,
			"email_send_per_ip_max":    10,
		}, file.Mail},
		{"sms", map[string]any{
			"phone_login":            false,
			"sms_code_timeout":       5,
			"sms_send_window_second": 60,
			"sms_send_per_phone_max": 1,
			"sms_send_per_ip_max":    10,
		}, file.SMS},
		{"oauth", map[string]any{
			"qq_login": false,
		}, file.OAuth},
		{"captcha", map[string]any{
			"captcha":              false,
			"captcha_login":        true,
			"captcha_email_verify": true,
			"captcha_phone_verify": true,
			"captcha_register":     true,
		}, file.Captcha},
	}
	for _, check := range checks {
		keys := make([]string, 0, len(check.values))
		for key := range check.values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			got, ok := check.data[key]
			if !ok {
				t.Errorf("%s 段缺少默认值 %s（默认值没跟着字段搬过来）", check.section, key)
				continue
			}
			if got != check.values[key] {
				t.Errorf("%s.%s 的默认值应为 %v，实际 %v（搬迁不得改变生效值）",
					check.section, key, check.values[key], got)
			}
		}
	}
}

// TestMigrateLoginSectionDryRunDoesNotWrite 钉住"默认只报告"。
func TestMigrateLoginSectionDryRunDoesNotWrite(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := upsertPayload(db, SectionSite, legacySiteLoginPayload); err != nil {
		t.Fatalf("预置老载荷失败: %v", err)
	}

	result, err := svc.MigrateLoginSection(false)
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if !result.Changed() {
		t.Fatal("库里还有旧键时，报告必须说明发现了要搬的字段")
	}
	if result.Applied {
		t.Fatal("dry-run 不得标记为已写库")
	}
	if payload := readPayload(t, db, SectionSite); payload != legacySiteLoginPayload {
		t.Fatalf("dry-run 不得改动库里的 payload，实际 %s", payload)
	}
}

// TestMigrateLoginSectionMovesValuesAndIsIdempotent 是这条命令的核心用例：
// 搬过去的值要生效、目标段已有的键要保留目标值、第二次执行不改库。
func TestMigrateLoginSectionMovesValuesAndIsIdempotent(t *testing.T) {
	svc, db := newTestService(t, conf.RuntimeDefaults{})
	if err := upsertPayload(db, SectionSite, legacySiteLoginPayload); err != nil {
		t.Fatalf("预置老 site 载荷失败: %v", err)
	}
	// mail 段已有（带密钥），用来证明搬到"已存在的段"时其它键原样保留。
	if err := upsertPayload(db, SectionMail, `{"secret_id":"sid","secret_key":"enc:v1:kept","from_address":"a@b.c"}`); err != nil {
		t.Fatalf("预置 mail 段失败: %v", err)
	}
	// captcha 段已有 captcha_login=false：按"目标段为准"跳过，不覆盖。
	if err := upsertPayload(db, SectionCaptcha, `{"driver":"math","captcha_login":false}`); err != nil {
		t.Fatalf("预置 captcha 段失败: %v", err)
	}

	result, err := svc.MigrateLoginSection(true)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if !result.Applied || !result.Changed() {
		t.Fatalf("有旧键时应写库，实际 applied=%v changed=%v", result.Applied, result.Changed())
	}
	if result.SkippedCount() != 1 {
		t.Fatalf("应恰好跳过 1 个键（captcha_login 目标段已有），实际 %d", result.SkippedCount())
	}

	sitePayload := readPayload(t, db, SectionSite)
	for _, key := range []string{
		"email_login", "phone_login", "qq_login", "captcha", "captcha_login",
		"email_code_timeout", "sms_send_window_second",
	} {
		if strings.Contains(sitePayload, `"`+key+`"`) {
			t.Fatalf("site.login 里不得再留 %q（用户要求物理删除），实际 %s", key, sitePayload)
		}
	}
	for _, key := range []string{"username_pwd_login", "login_fail_window_minute"} {
		if !strings.Contains(sitePayload, `"`+key+`"`) {
			t.Fatalf("没被迁走的 %q 应原样留在 site.login，实际 %s", key, sitePayload)
		}
	}

	// 目标段：搬进去的值 + 原有的其它键都要在。
	for _, want := range []struct {
		name    SectionName
		needles []string
	}{
		{SectionMail, []string{`"email_login":true`, `"email_code_timeout":7`, `"secret_key":"enc:v1:kept"`}},
		{SectionSMS, []string{`"phone_login":true`, `"sms_send_window_second":30`}},
		{SectionOAuth, []string{`"qq_login":true`}},
		{SectionCaptcha, []string{`"captcha":true`, `"captcha_login":false`, `"driver":"math"`}},
	} {
		payload := readPayload(t, db, string(want.name))
		for _, needle := range want.needles {
			if !strings.Contains(payload, needle) {
				t.Fatalf("%s 段应含 %s，实际 %s", want.name, needle, payload)
			}
		}
	}

	// 生效值：搬过去之后重新加载，读到的必须是管理员当年填的那一份。
	if err := svc.InitRuntimeConfig(); err != nil {
		t.Fatalf("迁移后重新加载失败: %v", err)
	}
	if !svc.GetRuntimeMail().EmailLogin || svc.GetRuntimeMail().EmailCodeTimeout != 7 {
		t.Fatalf("邮箱段生效值不对: %+v", svc.GetRuntimeMail())
	}
	if !svc.GetRuntimeSMS().PhoneLogin || svc.GetRuntimeSMS().SmsSendWindowSecond != 30 {
		t.Fatalf("短信段生效值不对: %+v", svc.GetRuntimeSMS())
	}
	if !svc.GetRuntimeOAuth().QQLogin {
		t.Fatal("oauth.qq_login 应为 true")
	}
	captcha := svc.GetRuntimeCaptcha()
	if !captcha.Captcha || captcha.CaptchaLogin || !captcha.CaptchaRegister {
		t.Fatalf("captcha 段生效值不对（captcha_login 应保留目标段的 false）: %+v", captcha)
	}

	// 幂等：第二次执行必须"无需迁移"，且不改任何一个字节。
	before := readPayload(t, db, SectionSite)
	second, err := svc.MigrateLoginSection(true)
	if err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}
	if second.Changed() || second.Applied {
		t.Fatalf("第二次执行应报告无需迁移且不写库，实际 changed=%v applied=%v",
			second.Changed(), second.Applied)
	}
	if after := readPayload(t, db, SectionSite); after != before {
		t.Fatalf("第二次执行不得改动 payload，前后不一致:\n前 %s\n后 %s", before, after)
	}
}

// TestLoginSectionMigrationReportLines 钉住摘要的措辞：搬了哪些、跳过哪些、要不要 -apply。
func TestLoginSectionMigrationReportLines(t *testing.T) {
	noop := strings.Join(LoginSectionMigrationReportLines(LoginSectionMigration{}, true), "\n")
	if !strings.Contains(noop, "无需迁移") {
		t.Fatalf("没有旧键时应报告无需迁移，实际：%s", noop)
	}

	result := LoginSectionMigration{
		Moves: []LoginSectionMove{
			{Key: "email_login", Target: SectionMail, Value: "true"},
			{Key: "captcha_login", Target: SectionCaptcha, Value: "true", KeptTarget: true},
		},
	}
	dryRun := strings.Join(LoginSectionMigrationReportLines(result, false), "\n")
	for _, must := range []string{"email_login", "mail", "true", "captcha_login", "跳过", "-apply"} {
		if !strings.Contains(dryRun, must) {
			t.Fatalf("摘要应包含 %q，实际：%s", must, dryRun)
		}
	}
	if strings.Contains(dryRun, "已写库") {
		t.Fatalf("dry-run 的摘要不得声称已写库，实际：%s", dryRun)
	}

	result.Applied = true
	applied := strings.Join(LoginSectionMigrationReportLines(result, true), "\n")
	if !strings.Contains(applied, "已写库") {
		t.Fatalf("-apply 且在摘要里明确说已写库，实际：%s", applied)
	}
}

// legacySiteLoginPayload 是升级前的 site 段形状：被迁走的键都还挂在 login 里。
const legacySiteLoginPayload = `{"site_info":{"beian":"粤ICP备1号"},"login":{` +
	`"username_pwd_login":true,"login_fail_window_minute":15,` +
	`"email_login":true,"email_code_timeout":7,` +
	`"phone_login":true,"sms_send_window_second":30,` +
	`"qq_login":true,` +
	`"captcha":true,"captcha_login":true,"captcha_register":true}}`

// jsonMarshalSite 把空 site 段序列化后按子块拆开，用于断言存储形状。
func jsonMarshalSite(t *testing.T) (map[string]map[string]json.RawMessage, error) {
	t.Helper()
	raw, err := json.Marshal(conf.Site{Login: siteconf.Login{}})
	if err != nil {
		return nil, err
	}
	var sections map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	return sections, nil
}

// sortedKeys 返回对象里排序后的键名（对 json.RawMessage 与 yaml 解出的 any 都适用）。
func sortedKeys[T any](object map[string]T) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
