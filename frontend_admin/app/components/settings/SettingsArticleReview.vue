<script setup lang="ts">
/**
 * 文章审核设置 · 审核规则（`/site/article-review`）：机器判据的唯一配置入口。
 *
 * 2026-11-28 起这一页变成「两子页」的父分组，本组件是**审核规则**子页：
 *
 *   1. **内容审核** —— 文章免审核（提交后要不要进人工队列）；
 *   2. **文章质量评分** —— 总开关。关掉后三条评分产生路径全停、界面不再显示分数、
 *      闸门**强制关闭**（父子关系，保存时后端强写 false）；
 *   3. **发布质量闸门** —— 提交前先评分、木桶判定（ADR-0023）。依赖上一条；
 *   4. **AI 合规判定** —— 第③层模型判定的总开关 + 不可用 / 失败时的四档落点；
 *   5. **投稿配额** —— 一天能交几篇（ADR-0022）。
 *
 * 它写 site 段的 `article` 块，与「文章发布」页共用这一段，所以保存走**字段级补丁**
 * （块级替换会把兄弟页的改动覆盖回旧值）。命中词的处置不在本页 —— 它住
 * 「关键词命中处理」子页，写的是 content_filter 段。
 *
 * 两子页共用一个聚合端点 `PUT /site/article-review`：审核规则只传 article，
 * 命中处理只传 auto_dispose。
 */
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NSelect,
  NSwitch,
  useMessage,
} from "naive-ui";

import {
  getSiteRuntimeConfig,
  updateArticleReviewConfig,
} from "~/services/admin";
import type {
  AiFailureAction,
  QualityGateConfig,
  SiteRuntimeConfig,
} from "~/types/api";
import { applySiteBlockFieldPatch } from "~/utils/siteSectionSave";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const config = ref({} as SiteRuntimeConfig);
const loaded = ref(false);

/**
 * 本页拥有的 `article` 块内字段（保存时只覆盖这些）。
 *
 * 2026-11-28：删掉废弃的 quality_auto_release，加进评分与 AI 合规两个新开关
 * 及失败落点。字段清单是**防覆盖的契约**：少写一个，那次保存就会把兄弟页
 * （文章发布页）的值写回旧值。
 */
const OWNED_ARTICLE_FIELDS = [
  "skip_examining",
  "submit_daily_max",
  "quality_gate",
  "score_enabled",
  "ai_compliance_enabled",
  "ai_failure_action",
] as const;

// 闸门默认值与后端 site_default_settings.yaml 一致：管理后台只负责改，
// 不发明第二套默认值。实际放行线 = pass_score + gray_band_width。
const DEFAULT_QUALITY_GATE: QualityGateConfig = {
  enabled: false,
  pass_score: 50,
  gray_band_width: 2,
  band_poor_max: 30,
  band_unqualified_max: 59,
  band_normal_max: 80,
  band_good_max: 89,
};

const qualityGate = ref<QualityGateConfig>({ ...DEFAULT_QUALITY_GATE });
const submitDailyMax = ref(15);
/** 文章质量评分总开关（默认开）。 */
const scoreEnabled = ref(true);
/** AI 合规判定开关（默认开）。 */
const aiComplianceEnabled = ref(true);
/** AI 不可用 / 失败时的落点（默认 manual）。 */
const aiFailureAction = ref<AiFailureAction>("manual");

/** 失败落点的下拉项：四档各自的代价必须写在描述里，否则没人会选对。 */
const AI_FAILURE_OPTIONS: Array<{
  label: string;
  value: AiFailureAction;
  description: string;
}> = [
  {
    label: "转人工",
    value: "manual",
    description: "进审核队列等人处理。作者要等，但误判不会落在他头上。",
  },
  {
    label: "保持待可见，次日重投",
    value: "hold",
    description: "读者看不到，第二天自动重试。不需要人，但要等到明天。",
  },
  {
    label: "退回草稿",
    value: "draft",
    description: "作者会收到「审核服务暂时不可用」的通知，可立刻重投。",
  },
  {
    label: "直接放行",
    value: "release",
    description: "临时兜底：文章没有合规结论，事后自动撤下对它不生效。",
  },
];

/** 平均评分关掉时闸门必然关（后端强写），所以这里置灰而不是隐藏。 */
const gateEditable = computed(() => scoreEnabled.value);

// 实际放行线在界面上直接算给运营看：只写"及格线"会让人以为 50 分就能过。
const publishLine = computed(
  () => qualityGate.value.pass_score + qualityGate.value.gray_band_width,
);

async function load() {
  loading.value = true;
  try {
    config.value = await getSiteRuntimeConfig();
    // 闸门与配额是后加的配置段：老库合并默认值之前这一段可能整块缺失，
    // 因此显式兜一层，表单不会因为 undefined 整页炸掉。
    qualityGate.value = {
      ...DEFAULT_QUALITY_GATE,
      ...(config.value.article?.quality_gate ?? {}),
    };
    submitDailyMax.value = config.value.article?.submit_daily_max ?? 15;
    // 两个开关的默认值是 true：后端 site_default_settings.yaml 显式写 true，
    // 这里用 ?? true 兜一层，避免老响应体缺字段时把它们误当作"关掉"。
    scoreEnabled.value = config.value.article?.score_enabled ?? true;
    aiComplianceEnabled.value = config.value.article?.ai_compliance_enabled ?? true;
    aiFailureAction.value =
      (config.value.article?.ai_failure_action as AiFailureAction | undefined) ??
      "manual";
    loaded.value = true;
  } catch {
    message.error("文章审核设置加载失败");
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    if (!config.value.article) {
      message.error("配置未加载完成，请刷新页面后重试");
      return;
    }
    config.value.article.quality_gate = { ...qualityGate.value };
    config.value.article.submit_daily_max = submitDailyMax.value;
    config.value.article.score_enabled = scoreEnabled.value;
    config.value.article.ai_compliance_enabled = aiComplianceEnabled.value;
    config.value.article.ai_failure_action = aiFailureAction.value;
    // 闸门的联动**由后端做**（normalizeArticle 在评分关时强写 false）。
    // 前端这里只负责置灰，不替后端改值 —— 否则绕过页面写配置的人会造出
    // "评分关、闸门开"的矛盾值。
    // 读-改-写：取最新整段 → 只覆盖本页字段 → 写回。
    const latest = await getSiteRuntimeConfig();
    const merged = applySiteBlockFieldPatch(
      latest,
      config.value,
      "article",
      OWNED_ARTICLE_FIELDS,
    );
    // 走聚合端点：本页只传 article 一段（另半段由「关键词命中处理」子页负责）。
    await updateArticleReviewConfig({ article: merged.article });
    message.success("文章审核设置已保存");
  } catch (error) {
    message.error(
      error instanceof Error ? `保存失败：${error.message}` : "保存失败",
    );
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="info" :bordered="false" class="mb-4">
      这一页管「审不审、审多严、一天能交几篇」，是机器判据的总闸。它与「文章发布」页
      写同一段配置，改完点一次保存即可；两边同时开着也不会互相覆盖。命中词的处置
      在旁边的「关键词命中处理」子页。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="内容审核" size="small">
        <NFormItem label="文章免审核">
          <NSwitch v-model:value="config.article.skip_examining" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          评论不再有独立审核（票
          400）：评论一律发布，命中词由全站内容过滤替换成星号 并进「审核 →
          关键词风险」。文章那条链路的判定顺序保持不变。
        </NAlert>
      </NCard>

      <!-- 文章质量评分（2026-11-28）：它是闸门的父开关，关掉后连闸门一起失效。 -->
      <NCard class="admin-card mb-4" title="文章质量评分" size="small">
        <NFormItem label="开启质量评分">
          <NSwitch v-model:value="scoreEnabled" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          关掉之后<strong>三条产生路径一起停</strong>：提交时同步评分、后台重评、发布判定补算。
          读者站与写稿页不再显示任何分数，评分接口统一返回「站点已关闭文章质量评分」。
          历史评分仍在库中，重新打开即恢复显示。
        </NAlert>
        <NAlert type="warning" :bordered="false" class="mb-4">
          代价：审核队列的「风险」列由分数派生（低分 / 维度塌陷 / 篇幅异常都看分），
          评分一停，同步风险扫描不再产生结论，风险列只能显示「未评估」。
        </NAlert>
        <NFormItem label="关闭后闸门同步失效">
          <NSwitch :value="!scoreEnabled" disabled />
        </NFormItem>
      </NCard>

      <!-- AI 合规判定（2026-11-28）：第③层模型判定的总闸 + 失败落点。 -->
      <NCard class="admin-card mb-4" title="AI 合规判定" size="small">
        <NFormItem label="启用 AI 合规判定">
          <NSwitch v-model:value="aiComplianceEnabled" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          关掉之后文章不再走第③层模型判定：质量关过线即可见。它跳过的是「等 AI 判定」
          这一步，不是「判定失败也放行」—— 拿不到分数与质量不过两条路径照旧不放行。
          与「文章免审核」的区别是：那是临时放行，这是常态配置。
        </NAlert>
        <NAlert v-if="!aiComplianceEnabled" type="warning" :bordered="false" class="mb-4">
          AI 合规判定已关闭：质量过线的文章会直接对读者可见，且不留合规结论
          （审核页「合规」列显示「未判定」），事后「AI 判违规自动撤下」对它不生效。
        </NAlert>
        <NFormItem label="判定不可用 / 失败时的落点">
          <NSelect
            v-model:value="aiFailureAction"
            class="admin-field--select"
            :options="AI_FAILURE_OPTIONS"
            :disabled="!aiComplianceEnabled"
          />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          只支配「这次调用失败」（AI 未装配、接口报错重试后仍失败）。模型判「存疑」或
          「低风险违规」是<strong>结论</strong>不是失败，仍转人工；额度用完固定等次日重投，也不看这一项。
          {{ AI_FAILURE_OPTIONS.find((o) => o.value === aiFailureAction)?.description }}
        </NAlert>
      </NCard>

      <NCard class="admin-card mb-4" title="投稿配额" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          每个普通用户每自然日可提交多少次。计量的是<strong>提交动作</strong>：被打回也计一次
          （每次都真的产生一条审核任务）；因系统不可用而失败的提交不占额度；管理员不受此上限约束。
        </NAlert>
        <NFormItem label="每日投稿上限">
          <NInputNumber
            v-model:value="submitDailyMax"
            class="admin-field--number"
            :min="1"
            :max="9999"
          />
        </NFormItem>
      </NCard>

      <!-- 发布质量闸门（票 306）：开启后提交前先评分，不达标不放行。
           ADR-0023 起是<strong>木桶</strong>判定：总分与六个维度都要过线。 -->
      <NCard class="admin-card mb-4" title="发布质量闸门" size="small">
        <NAlert type="warning" :bordered="false" class="mb-4">
          开启前必须先让 AI
          可用（总闸打开且至少配置了一条启用模型），否则保存会被拒绝——
          闸门失败关闭，叠上 AI 不可用就等于"所有投稿都被拒"。
        </NAlert>
        <NAlert type="info" :bordered="false" class="mb-4">
          判定是<strong>木桶</strong>：总分要达到 {{ publishLine }} 分（及格线 + 灰带宽度），
          并且六个维度（清晰度 / 结构性 / 完整度 / 可读性 / 说服力 / 语言规范）
          <strong>每一项</strong>都要达到及格线 {{ qualityGate.pass_score }} 分。
          任何一项不够都不能发布，总分再高也不行——否则长板会把短板抵消掉，
          两行话也能凑出及格的加权分。
        </NAlert>
        <NFormItem label="开启质量闸门">
          <NSwitch v-model:value="qualityGate.enabled" :disabled="!gateEditable" />
        </NFormItem>
        <NAlert v-if="!gateEditable" type="warning" :bordered="false" class="mb-4">
          文章质量评分已关闭，闸门没有判据可用，因此保存时会随评分一起被关闭。
          要重新开闸门，请先打开上面的「开启质量评分」，再回来打开它。
        </NAlert>
        <NFormItem label="及格线（同时是每个维度的下限）">
          <NInputNumber
            v-model:value="qualityGate.pass_score"
            class="admin-field--number"
            :min="0"
            :max="100"
          />
        </NFormItem>
        <NFormItem label="灰带宽度">
          <NInputNumber
            v-model:value="qualityGate.gray_band_width"
            class="admin-field--number"
            :min="0"
            :max="20"
          />
        </NFormItem>
        <NFormItem label="档界：较差上限">
          <NInputNumber
            v-model:value="qualityGate.band_poor_max"
            class="admin-field--number"
            :min="0"
            :max="100"
          />
        </NFormItem>
        <NFormItem label="档界：不合格上限">
          <NInputNumber
            v-model:value="qualityGate.band_unqualified_max"
            class="admin-field--number"
            :min="0"
            :max="100"
          />
        </NFormItem>
        <NFormItem label="档界：常规上限">
          <NInputNumber
            v-model:value="qualityGate.band_normal_max"
            class="admin-field--number"
            :min="0"
            :max="100"
          />
        </NFormItem>
        <NFormItem label="档界：优质上限">
          <NInputNumber
            v-model:value="qualityGate.band_good_max"
            class="admin-field--number"
            :min="0"
            :max="100"
          />
        </NFormItem>
      </NCard>

      <!-- 「质量达标自动放行」已废弃（2026-11-28 / ADR-0037 D14）：它的语义
           正好就是上一条「AI 合规判定」关掉的效果，留着等于同一件事两个开关。 -->

      <NButton type="primary" :loading="saving" @click="save"
        >保存文章审核设置</NButton
      >
    </NForm>
    <div v-else class="admin-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>
  </div>
</template>
