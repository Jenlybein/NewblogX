<script setup lang="ts">
/**
 * 文章审核设置（`/site/article-review`）：审核规则的唯一配置入口（2026-11-27）。
 *
 * 本页从「文章发布」拆出（ADR-0029 第三版）。三块内容按"审核链路读一遍"排列：
 *
 *   1. **内容审核** —— 免审开关（提交后要不要进人工队列）；
 *   2. **投稿配额** —— 一天能交几篇（配额按自然日重置，ADR-0022）；
 *   3. **发布质量闸门** —— 提交前先评分，木桶判定（总分与六维都过线，ADR-0023）。
 *
 * 它们与「文章发布」页（谁能发、发出来对谁可见）写同一个 site 段的 `article` 块，
 * 所以保存必须走**字段级补丁**：块级替换会把兄弟页的改动覆盖回旧值。
 *
 * 后续在这里加的两项（2026-11-27 待做）：
 *   - 质量达标自动放行：机器判定 clean 且质量过线时直接转 visible，跳过人工；
 *   - 命中词自动处置档位：拦截档命中即拒、观察档命中只打星。
 */
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NSwitch,
  useMessage,
} from "naive-ui";

import {
  getSiteRuntimeConfig,
  updateSiteRuntimeConfig,
} from "~/services/admin";
import type { QualityGateConfig, SiteRuntimeConfig } from "~/types/api";
import { applySiteBlockFieldPatch } from "~/utils/siteSectionSave";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const config = ref({} as SiteRuntimeConfig);
const loaded = ref(false);

/** 本页拥有的 `article` 块内字段（保存时只覆盖这些）。 */
const OWNED_ARTICLE_FIELDS = [
  "skip_examining",
  "submit_daily_max",
  "quality_gate",
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
    // 读-改-写：取最新整段 → 只覆盖本页字段 → 写回。
    const latest = await getSiteRuntimeConfig();
    const merged = applySiteBlockFieldPatch(
      latest,
      config.value,
      "article",
      OWNED_ARTICLE_FIELDS,
    );
    await updateSiteRuntimeConfig(merged);
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
      这一页管「审不审、审多严、一天能交几篇」。这三块与「文章发布」页写同一段配置，
      改完点一次保存即可；两边同时开着也不会互相覆盖。
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

      <NCard class="admin-card mb-4" title="投稿配额" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          每个普通用户每自然日可提交多少次。计量的是**提交动作**：被打回也计一次
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
           ADR-0023 起是**木桶**判定：总分与六个维度都要过线。 -->
      <NCard class="admin-card mb-4" title="发布质量闸门" size="small">
        <NAlert type="warning" :bordered="false" class="mb-4">
          开启前必须先让 AI
          可用（总闸打开且至少配置了一条启用模型），否则保存会被拒绝——
          闸门失败关闭，叠上 AI 不可用就等于"所有投稿都被拒"。
        </NAlert>
        <NAlert type="info" :bordered="false" class="mb-4">
          判定是**木桶**：总分要达到 {{ publishLine }} 分（及格线 + 灰带宽度），
          并且六个维度（清晰度 / 结构性 / 完整度 / 可读性 / 说服力 / 语言规范）
          **每一项**都要达到及格线 {{ qualityGate.pass_score }} 分。
          任何一项不够都不能发布，总分再高也不行——否则长板会把短板抵消掉，
          两行话也能凑出及格的加权分。
        </NAlert>
        <NFormItem label="开启质量闸门">
          <NSwitch v-model:value="qualityGate.enabled" />
        </NFormItem>
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

      <NButton type="primary" :loading="saving" @click="save"
        >保存文章审核设置</NButton
      >
    </NForm>
    <div v-else class="admin-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>
  </div>
</template>
