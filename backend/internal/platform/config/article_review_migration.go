package config

import (
	"encoding/json"
	"sort"
	"strings"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/apperr"

	"gorm.io/gorm"
)

// ArticleReviewMigration 是「文章审核设置」这一批开关的数据迁移结果（2026-11-28 / ADR-0037）。
//
// 迁移做三件事（全部幂等）：
//
//  1. `site.article.quality_auto_release` 删除；按它的现值写 `ai_compliance_enabled`
//     （`true → false`，`false / 缺键 → true`）。旧开关的语义正是"质量过线就别等
//     AI 判定"，等价于关掉 AI 合规判定。
//  2. `site.article` 缺 `score_enabled` / `ai_failure_action` 时补默认值
//     （`true` / `manual`）。**必须补**：结构体零值恰好都是"关"与"空串"，
//     缺键会静默把线上行为翻成"评分关闭 + 落点未知"。
//  3. `content_filter.auto_dispose` 的旧值 `mask` 改写为 `release`（同义，只是改个名）。
//
// 为什么放在 `-db` 里而不是只给一条手动命令：这三种改动都只改**数据**、都可重入，
// 而 `-db` 是每次部署都会跑的那一步 —— 放在这里意味着"新代码配旧数据"这个窗口
// 被关掉。它与 `-db` 原有的"只做加法"不冲突：不新增列、不改类型。
type ArticleReviewMigration struct {
	// SiteChanged 为真表示 site 段被改写（删了旧键或补了新键）。
	SiteChanged bool
	// ContentFilterChanged 为真表示 content_filter 段被改写（mask → release）。
	ContentFilterChanged bool
	// QualityAutoRelease 是读到的旧值（缺键时为 nil），只用于报告。
	QualityAutoRelease *bool
	// Applied 为真表示这次真的写了库；dry-run 下永远为假。
	Applied bool
}

// Changed 判断这次有没有需要落库的改动 —— 幂等判定的唯一口径。
func (m ArticleReviewMigration) Changed() bool {
	return m.SiteChanged || m.ContentFilterChanged
}

// MigrateArticleReview 检查（并在 apply 时执行）「文章审核设置」的数据迁移。
//
// apply 为假时只算计划、不写库（与 -migrate-runtime-config 同一条口径）。
// 两个段在一个事务里写：半迁移（site 段删了旧键、content_filter 还是 mask）
// 是最难排查的状态，必须要么全成要么全不成。
func (s *RuntimeConfigService) MigrateArticleReview(apply bool) (ArticleReviewMigration, error) {
	if s.db == nil {
		return ArticleReviewMigration{}, apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	if !s.db.Migrator().HasTable(&runtimeConfigRow{}) {
		// 表还没有（首次部署）时没有历史数据可迁，这是"跳过"不是"失败"。
		return ArticleReviewMigration{}, nil
	}

	var result ArticleReviewMigration
	err := s.db.Transaction(func(tx *gorm.DB) error {
		payloads, err := loadPayloads(tx)
		if err != nil {
			return err
		}
		sitePayload, filterPayload, plan, err := planArticleReviewMigration(payloads)
		if err != nil {
			return err
		}
		result.SiteChanged = plan.siteChanged
		result.ContentFilterChanged = plan.filterChanged
		result.QualityAutoRelease = plan.qualityAutoRelease
		if !apply || !plan.changed() {
			return nil
		}
		if plan.siteChanged {
			if err := upsertPayload(tx, string(SectionSite), sitePayload); err != nil {
				return err
			}
		}
		if plan.filterChanged {
			if err := upsertPayload(tx, string(SectionContentFilter), filterPayload); err != nil {
				return err
			}
		}
		result.Applied = true
		return nil
	})
	if err != nil {
		return ArticleReviewMigration{}, err
	}
	return result, nil
}

// articleReviewPlan 是计划阶段的产物。
type articleReviewPlan struct {
	siteChanged        bool
	filterChanged      bool
	qualityAutoRelease *bool
}

func (p articleReviewPlan) changed() bool { return p.siteChanged || p.filterChanged }

// planArticleReviewMigration 从库里的原始 payload 算出迁移计划（纯函数，便于断言）。
//
// 返回改写后的两个 payload 原文（没有改动时为空串）。键顺序保持稳定：
// Go 的 map 遍历是随机的，但这里只按需改键值、最后用 seqOrder 重排，
// 保证"同一个输入两次执行产出同一份字节"（幂等可以逐字验证）。
func planArticleReviewMigration(payloads map[string]string) (string, string, articleReviewPlan, error) {
	var plan articleReviewPlan

	sitePayload, siteChanged, err := planSiteArticle(payloads, &plan)
	if err != nil {
		return "", "", plan, err
	}
	filterPayload, filterChanged, err := planContentFilterDispose(payloads, &plan)
	if err != nil {
		return "", "", plan, err
	}
	plan.siteChanged = siteChanged
	plan.filterChanged = filterChanged
	return sitePayload, filterPayload, plan, nil
}

// planSiteArticle 处理 site.article 段：删旧键、补新键。
func planSiteArticle(payloads map[string]string, plan *articleReviewPlan) (string, bool, error) {
	raw, ok := payloads[string(SectionSite)]
	if !ok {
		return "", false, nil
	}
	top, err := decodeRawObject(raw)
	if err != nil {
		return "", false, err
	}
	articleRaw, ok := top["article"]
	if !ok {
		// 连 article 块都没有：启动路径的种子会按默认值补齐这一段，这里无事可做。
		return "", false, nil
	}
	article, err := decodeRawObject(string(articleRaw))
	if err != nil {
		return "", false, err
	}

	changed := false

	// ① 旧开关：读现值 → 决定 ai_compliance_enabled → 删键。
	if old, exists := article["quality_auto_release"]; exists {
		var enabled bool
		if err := json.Unmarshal(old, &enabled); err == nil {
			plan.qualityAutoRelease = &enabled
			// 旧开关为 true = "质量过线就别等 AI 判定" ⇒ 关掉 AI 合规判定。
			// false / 无法解析 ⇒ 保持 AI 合规判定开启（改造前的等价行为）。
			article["ai_compliance_enabled"] = jsonBool(!enabled)
			changed = true
		}
		delete(article, "quality_auto_release")
		changed = true
	}

	// ② 缺键补默认值。缺键时结构体零值 = false / ""，会把线上行为翻成
	//    "评分关闭 + 落点未知"，而站点默认是"全开"。
	if _, exists := article["ai_compliance_enabled"]; !exists {
		article["ai_compliance_enabled"] = jsonBool(true)
		changed = true
	}
	if _, exists := article["score_enabled"]; !exists {
		article["score_enabled"] = jsonBool(true)
		changed = true
	}
	if actionRaw, exists := article["ai_failure_action"]; !exists {
		article["ai_failure_action"] = jsonString(siteconf.AIFailureManual)
		changed = true
	} else {
		var action string
		if err := json.Unmarshal(actionRaw, &action); err != nil || action == "" {
			article["ai_failure_action"] = jsonString(siteconf.AIFailureManual)
			changed = true
		} else if !isAIFailureAction(action) {
			// 非法值不静默改成默认：那会掩盖"有人手改过库"这件事。
			// 保存路径会拒绝这种值，这里原样留着，让下一次保存去纠正它。
			_ = action
		}
	}

	if !changed {
		return "", false, nil
	}

	marshaledArticle, err := marshalRawObject(article)
	if err != nil {
		return "", false, err
	}
	top["article"] = marshaledArticle
	out, err := marshalRawObject(top)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

// planContentFilterDispose 把 auto_dispose 里的旧值 mask 改写为 release，并补齐 comment_action。
func planContentFilterDispose(payloads map[string]string, plan *articleReviewPlan) (string, bool, error) {
	raw, ok := payloads[string(SectionContentFilter)]
	if !ok {
		return "", false, nil
	}
	top, err := decodeRawObject(raw)
	if err != nil {
		return "", false, err
	}
	disposeRaw, ok := top["auto_dispose"]
	if !ok {
		return "", false, nil
	}
	dispose, err := decodeRawObject(string(disposeRaw))
	if err != nil {
		return "", false, err
	}

	changed := false
	for _, key := range []string{"block_action", "watch_action", "comment_action"} {
		valueRaw, exists := dispose[key]
		if !exists {
			// 评论档是 2026-12-01 新增的：老 payload 没有这个键，缺键补 release
			// 是刻意的 —— 零值（空串）在评论侧同样是"打星照发"，但显式写进去之后
			// 后台那一页才有值可回显，不必等运营先保存一次。
			if key == "comment_action" {
				dispose[key] = jsonString(conf.ContentFilterActionRelease)
				changed = true
			}
			continue
		}
		var action string
		if err := json.Unmarshal(valueRaw, &action); err != nil {
			continue
		}
		// 旧值 mask 一律读成 release（评论档同理：老库只有 block/watch 两个键）。
		if action == legacyDisposeMask {
			dispose[key] = jsonString(conf.ContentFilterActionRelease)
			changed = true
		}
	}
	if !changed {
		return "", false, nil
	}

	marshaledDispose, err := marshalRawObject(dispose)
	if err != nil {
		return "", false, err
	}
	top["auto_dispose"] = marshaledDispose
	out, err := marshalRawObject(top)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

// legacyDisposeMask 是改造前"只打星"的取值（2026-11-28 起被 release 取代）。
const legacyDisposeMask = conf.ContentFilterActionLegacyMask

// isAIFailureAction 判断取值是否在四档之内（报告与校验共用一份判据）。
func isAIFailureAction(action string) bool {
	for _, allowed := range siteconf.AIFailureActions {
		if action == allowed {
			return true
		}
	}
	return false
}

// jsonBool / jsonString 是构造 RawMessage 的两个小工具。
func jsonBool(v bool) json.RawMessage {
	if v {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}

func jsonString(v string) json.RawMessage {
	encoded, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

// marshalRawObject 把一个键值对重新编码成 JSON 对象，并**按键名排序**。
//
// 排序是幂等的一部分：map 遍历顺序随机，不排序的话同一次迁移两次执行的输出
// 字节不同，"再跑一次结果不变"就只能靠语义判断、没法逐字比对。
func marshalRawObject(obj map[string]json.RawMessage) (json.RawMessage, error) {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString("{")
	for index, key := range keys {
		if index > 0 {
			builder.WriteString(",")
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		builder.Write(encodedKey)
		builder.WriteString(":")
		builder.Write(obj[key])
	}
	builder.WriteString("}")
	return json.RawMessage(builder.String()), nil
}
