package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"myblogx/internal/apperr"
)

// site.login 的字段按通道拆分（票 票 481）之后的一次性数据搬迁。
//
// # 为什么必须搬
//
// 被拆走的键已经从 site.Login 结构体里删掉，而"读"路径靠 mergeJSON 把库里的
// 载荷合并到默认值上 —— 结构体里没有的键只会作为未知键被 json 解码忽略。
// 也就是说，管理员当年在后台填的 email_login: true 会**静默失效**并回落到默认值，
// 表现与"后台改了没反应"一模一样，没有任何日志。用户明确要求不留兼容层
// （结构体里不保留 deprecated 字段），所以只能把老值搬到新位置。
//
// # 为什么是 CLI 手动跑而不是启动时自动迁移
//
// 它会改写 site 与最多四个目标段的 payload。放进启动路径意味着一次误判会让所有
// 副本同时改同一份配置，且没有"先看一眼要改什么"的机会；CLI 默认只报告、
// 带 -apply 才写库，运维可以先核对摘要再决定。
//
// # 幂等
//
// 判据只有一条：库里 site 段 payload 的 login 子对象里还有没有这些键。
// 搬完就把它们从 site.login 里物理删掉，所以第二次执行必然是"无需迁移"且不写库。

// loginSectionFieldMigration 是一条搬迁关系：site.login 的一个键 → 目标段。
type loginSectionFieldMigration struct {
	Key    string
	Target SectionName
}

// loginSectionMigrations 是「site.login 的这些键该搬去哪一段」的唯一清单。
//
// 为什么写成显式表而不是靠反射从结构体反推：反射只能告诉我们"现在字段在哪一段"，
// 说不出"它以前在哪一段、键名有没有改过"，而这段代码要处理的正是历史数据。
// 表里漏一个键，那个键的旧值就会在升级时静默丢失 —— 所以
// TestLoginSectionMigrationsCoverAllMovedFields 把十七项逐个钉住。
var loginSectionMigrations = []loginSectionFieldMigration{
	// 邮箱验证码通道（mail 段）。
	{Key: "email_login", Target: SectionMail},
	{Key: "email_register", Target: SectionMail},
	{Key: "email_code_timeout", Target: SectionMail},
	{Key: "email_send_window_second", Target: SectionMail},
	{Key: "email_send_per_email_max", Target: SectionMail},
	{Key: "email_send_per_ip_max", Target: SectionMail},
	// 手机号验证码通道（sms 段）。
	{Key: "phone_login", Target: SectionSMS},
	{Key: "sms_code_timeout", Target: SectionSMS},
	{Key: "sms_send_window_second", Target: SectionSMS},
	{Key: "sms_send_per_phone_max", Target: SectionSMS},
	{Key: "sms_send_per_ip_max", Target: SectionSMS},
	// 第三方登录（oauth 段）。
	{Key: "qq_login", Target: SectionOAuth},
	// 图形验证码（captcha 段）：总开关 + 四个入口闸门。
	{Key: "captcha", Target: SectionCaptcha},
	{Key: "captcha_login", Target: SectionCaptcha},
	{Key: "captcha_email_verify", Target: SectionCaptcha},
	{Key: "captcha_phone_verify", Target: SectionCaptcha},
	{Key: "captcha_register", Target: SectionCaptcha},
}

// LoginSectionMove 是一条被处理掉的搬迁（含"目标段已有、旧值被丢弃"的那些）。
type LoginSectionMove struct {
	Key    string
	Target SectionName
	// Value 是 site.login 里那份旧值的 JSON 原文（摘要里直接打印，便于核对）。
	Value string
	// KeptTarget 为真表示目标段已经有这个键，按"目标段优先"保留目标值、
	// 丢弃 site.login 里的旧值（旧键照样物理删除，不留第二份事实源）。
	KeptTarget bool
}

// LoginSectionMigration 是一次搬迁/检查的结果。
type LoginSectionMigration struct {
	// Moves 是 site.login 里发现并处理掉的键（按迁移表顺序，稳定输出）。
	Moves []LoginSectionMove
	// Applied 为真表示这次真的写了库；dry-run 下永远为假。
	Applied bool
}

// Changed 判断这次有没有需要落库的改动 —— 幂等判定的唯一口径。
func (m LoginSectionMigration) Changed() bool { return len(m.Moves) > 0 }

// MoveCount 返回真正搬到目标段的键数（不含"目标段已有、旧值被丢弃"的）。
func (m LoginSectionMigration) MoveCount() int {
	count := 0
	for _, move := range m.Moves {
		if !move.KeptTarget {
			count++
		}
	}
	return count
}

// SkippedCount 返回因为"目标段已有该键"而跳过覆盖的键数。
func (m LoginSectionMigration) SkippedCount() int { return len(m.Moves) - m.MoveCount() }

// loginSectionPlan 是计划阶段的产物：报告要用的记录 + 需要写回的 payload。
type loginSectionPlan struct {
	moves []LoginSectionMove
	// sitePayload 是删掉被搬键之后的 site 段 payload；没有改动时为 ""。
	sitePayload string
	// targetPayloads 是**真的新增了键**的目标段 payload（只含 dirty 的段）。
	targetPayloads map[SectionName]string
}

// changed 判断计划里有没有要写库的东西。
func (p loginSectionPlan) changed() bool { return len(p.moves) > 0 }

// MigrateLoginSection 检查（并在 apply 时执行）site.login 的字段搬迁。
//
// apply 为假时只算计划、不写库（`-migrate-runtime-config` 的默认行为）；
// 为真时在一个事务里写 site 段与所有 dirty 的目标段 —— 半迁移（site 段删了键、
// 目标段没写进去）是最难排查的状态，必须要么全成要么全不成。
func (s *RuntimeConfigService) MigrateLoginSection(apply bool) (LoginSectionMigration, error) {
	if s.db == nil {
		return LoginSectionMigration{}, apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	if !s.db.Migrator().HasTable(&runtimeConfigRow{}) {
		return LoginSectionMigration{}, apperr.New(apperr.CodeRuntimeConfigTableMissing)
	}

	var result LoginSectionMigration
	err := s.db.Transaction(func(tx *gorm.DB) error {
		payloads, err := loadPayloads(tx)
		if err != nil {
			return err
		}
		plan, err := planLoginSectionMigration(payloads)
		if err != nil {
			return err
		}
		result.Moves = plan.moves
		if !apply || !plan.changed() {
			return nil
		}
		if err := upsertPayload(tx, string(SectionSite), plan.sitePayload); err != nil {
			return err
		}
		// 段的写回顺序不影响语义（各段一行、互不引用），排序只是让日志与
		// 出错时的第一条错误稳定可复现。
		for _, name := range sortedPayloadNames(plan.targetPayloads) {
			if err := upsertPayload(tx, string(name), plan.targetPayloads[name]); err != nil {
				return err
			}
		}
		result.Applied = true
		return nil
	})
	if err != nil {
		return LoginSectionMigration{}, err
	}
	return result, nil
}

// planLoginSectionMigration 从库里的原始 payload 算出搬迁计划（纯函数，便于断言）。
//
// 用 json.RawMessage 保留每个值的原始 JSON 文本：这些值里有布尔、也有整数，
// 来回过一遍 float64 虽然这里不会失真，但"搬过去的字节与原值一致"是更容易验证的
// 不变量；目标段 payload 里的其它键（包括密文密钥）也因此原样保留。
func planLoginSectionMigration(payloads map[string]string) (loginSectionPlan, error) {
	plan := loginSectionPlan{targetPayloads: make(map[SectionName]string, 4)}

	sitePayload, ok := payloads[string(SectionSite)]
	if !ok {
		// 库里连 site 段都没有：没有任何旧值可搬（种子由启动路径负责）。
		return plan, nil
	}
	siteTop, err := decodeRawObject(sitePayload)
	if err != nil {
		return plan, err
	}
	loginRaw, ok := siteTop["login"]
	if !ok {
		return plan, nil
	}
	login, err := decodeRawObject(string(loginRaw))
	if err != nil {
		return plan, err
	}

	// targets 缓存各目标段已解析的对象，避免同一个段被解析多次。
	targets := make(map[SectionName]map[string]json.RawMessage, 4)
	dirty := make(map[SectionName]bool, 4)

	for _, migration := range loginSectionMigrations {
		value, exists := login[migration.Key]
		if !exists {
			continue
		}
		targetTop, err := targetSectionObject(payloads, targets, migration.Target)
		if err != nil {
			return plan, err
		}
		move := LoginSectionMove{
			Key:    migration.Key,
			Target: migration.Target,
			Value:  strings.TrimSpace(string(value)),
		}
		if _, taken := targetTop[migration.Key]; taken {
			// 目标段已有该键：按"目标段为准"跳过，不覆盖管理员后来在新位置改的值。
			move.KeptTarget = true
		} else {
			targetTop[migration.Key] = value
			dirty[migration.Target] = true
		}
		// 无论覆盖与否，旧键都要物理删除：用户明确要求不留第二份事实源，
		// 留着它只会让"同一个开关有两个值"在下次排查时重新出现。
		delete(login, migration.Key)
		plan.moves = append(plan.moves, move)
	}

	if len(plan.moves) == 0 {
		return plan, nil
	}

	encodedLogin, err := json.Marshal(login)
	if err != nil {
		return plan, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	siteTop["login"] = encodedLogin
	if plan.sitePayload, err = encodeRawObject(siteTop); err != nil {
		return plan, err
	}
	for _, name := range sortedSectionNames(dirty) {
		payload, err := encodeRawObject(targets[name])
		if err != nil {
			return plan, err
		}
		plan.targetPayloads[name] = payload
	}
	return plan, nil
}

// targetSectionObject 取目标段的对象形式；段不存在或 payload 为空时得到一个空对象
// （写回时就成了"只含搬迁字段的一段"，其余字段由默认值在读取时补齐）。
func targetSectionObject(
	payloads map[string]string,
	cache map[SectionName]map[string]json.RawMessage,
	name SectionName,
) (map[string]json.RawMessage, error) {
	if cached, ok := cache[name]; ok {
		return cached, nil
	}
	object, err := decodeRawObject(payloads[string(name)])
	if err != nil {
		return nil, err
	}
	cache[name] = object
	return object, nil
}

// decodeRawObject 把一段 payload 解析成"键 → JSON 原文"的对象。
//
// 空串与 "null" 都按空对象处理：本表的 payload 是 NOT NULL，但历史上出现过
// 手工置空的行，把它当成"没有这段"比让整条迁移报错更符合运维预期。
func decodeRawObject(payload string) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage)
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "null" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	if out == nil {
		out = make(map[string]json.RawMessage)
	}
	return out, nil
}

// encodeRawObject 把对象序列化回 payload。
func encodeRawObject(object map[string]json.RawMessage) (string, error) {
	raw, err := json.Marshal(object)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return string(raw), nil
}

// sortedSectionNames 返回排序后的段名，让写库顺序与报告稳定可复现。
func sortedSectionNames(set map[SectionName]bool) []SectionName {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, string(name))
	}
	sort.Strings(names)
	out := make([]SectionName, 0, len(names))
	for _, name := range names {
		out = append(out, SectionName(name))
	}
	return out
}

// sortedPayloadNames 是 sortedSectionNames 针对"段 → payload"这张表的薄适配。
func sortedPayloadNames(payloads map[SectionName]string) []SectionName {
	set := make(map[SectionName]bool, len(payloads))
	for name := range payloads {
		set[name] = true
	}
	return sortedSectionNames(set)
}

// LoginSectionMigrationReportLines 把结果渲染成人类可读的摘要（纯函数，便于断言）。
//
// 措辞刻意逐条列出"哪个键从哪搬到哪、值是什么"，而不是只报一个总数：
// 这条命令会改写五段配置，运维需要能一眼看出"我原来那个 true 有没有跟过去"。
// 被跳过的键单独一节，因为"目标段已有该键"意味着 site.login 里那个值**没生效**，
// 这是唯一需要人工复核的情况。
func LoginSectionMigrationReportLines(result LoginSectionMigration, apply bool) []string {
	if !result.Changed() {
		return []string{
			"site.login → 各段：无需迁移（site.login 里没有被迁走的字段）",
		}
	}

	lines := []string{
		fmt.Sprintf("site.login → 各段：发现 %d 个已迁字段（搬移 %d，跳过 %d）",
			len(result.Moves), result.MoveCount(), result.SkippedCount()),
	}
	for _, move := range result.Moves {
		if move.KeptTarget {
			lines = append(lines, fmt.Sprintf("  跳过 %s：%s 段已有该键，保留目标段的值；site.login 里的旧值 %s 已删除",
				move.Key, move.Target, move.Value))
			continue
		}
		lines = append(lines, fmt.Sprintf("  搬移 %s: site.login → %s（值 %s）",
			move.Key, move.Target, move.Value))
	}
	if result.SkippedCount() > 0 {
		lines = append(lines, "⚠ 上面被跳过的键在 site.login 里的值没有生效：目标段的值优先。请核对那几项是否符合预期。")
	}
	if apply && result.Applied {
		lines = append(lines, "已写库。")
	} else {
		lines = append(lines, "以上只是报告：加 -apply 才会写库。")
	}
	return lines
}
