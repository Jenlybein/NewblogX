<script setup lang="ts">
import type { AiArticleScoringDimension, AiArticleScoringIssue, AiArticleScoringResponseData } from "~/types/api";
import { computed } from "vue";
import { NButton, NCard, NEmpty, NModal, NProgress, NTag } from "naive-ui";
import { formatDateTimeLabel } from "~/utils/format";

const props = defineProps<{
  show: boolean;
  loading: boolean;
  refreshing?: boolean;
  data: AiArticleScoringResponseData | null;
  title?: string;
  allowRefresh?: boolean;
  refreshLabel?: string;
  emptyText?: string;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
  refresh: [];
}>();

const dimensionMetaMap: Record<AiArticleScoringDimension["name"], { label: string; color: string; railColor: string }> = {
  clarity: { label: "清晰度", color: "#0f766e", railColor: "rgba(15, 118, 110, 0.14)" },
  structure: { label: "结构性", color: "#1d4ed8", railColor: "rgba(29, 78, 216, 0.12)" },
  completeness: { label: "完整度", color: "#7c3aed", railColor: "rgba(124, 58, 237, 0.12)" },
  readability: { label: "可读性", color: "#ca8a04", railColor: "rgba(202, 138, 4, 0.12)" },
  persuasiveness: { label: "说服力", color: "#dc2626", railColor: "rgba(220, 38, 38, 0.12)" },
  language: { label: "语言规范", color: "#0f766e", railColor: "rgba(15, 118, 110, 0.14)" },
};

const modalTitle = computed(() => props.title || "文章质量评分");
const totalScoreLabel = computed(() => {
  if (!props.data?.has_score) {
    return "--";
  }
  return String(props.data.total_score ?? "--");
});
const dimensions = computed(() => props.data?.dimensions || []);
const issues = computed(() => props.data?.main_issues || []);
const hasDetails = computed(() => Boolean(props.data?.overall_comment || issues.value.length || dimensions.value.some((item) => item.reason)));

function close() {
  emit("update:show", false);
}

function getDimensionMeta(name: AiArticleScoringDimension["name"]) {
  return dimensionMetaMap[name];
}

function resolveIssueAnchor(issue: AiArticleScoringIssue) {
  return issue.positions
    .map((item) => {
      if (!item.paragraph && !item.quote) return "";
      if (!item.paragraph) return `“${item.quote}”`;
      return `第 ${item.paragraph} 段 · “${item.quote}”`;
    })
    .filter(Boolean)
    .join(" / ");
}
</script>

<template>
  <NModal :show="show" class="article-score-modal" @update:show="emit('update:show', $event)">
    <NCard :title="modalTitle" :bordered="false" closable class="article-score-card" @close="close">
      <div v-if="loading" class="article-score-loading">
        <div class="article-score-loading__ring" />
        <span>{{ refreshing ? "正在重新获取最新评分…" : "正在读取评分内容…" }}</span>
      </div>

      <div v-else-if="!data?.has_score" class="article-score-empty">
        <NEmpty :description="emptyText || '当前还没有可用评分记录。'" />
      </div>

      <div v-else class="article-score-body">
        <section class="article-score-hero">
          <div>
            <div class="article-score-hero__eyebrow">QUALITY SCORE</div>
            <div class="article-score-hero__total">{{ totalScoreLabel }}</div>
            <div class="article-score-hero__meta">
              <NTag v-if="data.score_level" round size="small" :bordered="false" type="info">{{ data.score_level }}</NTag>
              <NTag v-if="data.article_type" round size="small" :bordered="false">{{ data.article_type }}</NTag>
              <span v-if="data.created_at" class="article-score-hero__time">
                {{ formatDateTimeLabel(data.created_at) }}
              </span>
            </div>
          </div>

          <div v-if="typeof data.ai_total_score === 'number'" class="article-score-hero__side">
            <span class="article-score-hero__side-label">AI 原始分</span>
            <strong>{{ data.ai_total_score }}</strong>
          </div>
        </section>

        <section class="article-score-grid">
          <article
            v-for="dimension in dimensions"
            :key="dimension.name"
            class="article-score-dimension">
            <div class="article-score-dimension__top">
              <strong>{{ getDimensionMeta(dimension.name).label }}</strong>
              <span>{{ dimension.score }}</span>
            </div>
            <NProgress
              type="line"
              :percentage="dimension.score"
              :height="8"
              :show-indicator="false"
              processing
              :color="getDimensionMeta(dimension.name).color"
              :rail-color="getDimensionMeta(dimension.name).railColor" />
            <p v-if="dimension.reason" class="article-score-dimension__reason">{{ dimension.reason }}</p>
          </article>
        </section>

        <section v-if="data.overall_comment" class="article-score-section">
          <div class="article-score-section__title">综合建议</div>
          <p class="article-score-section__comment">{{ data.overall_comment }}</p>
        </section>

        <section v-if="issues.length" class="article-score-section">
          <div class="article-score-section__title">主要问题</div>
          <div class="article-score-issues">
            <article v-for="(issue, index) in issues" :key="index" class="article-score-issue">
              <div class="article-score-issue__index">0{{ index + 1 }}</div>
              <div class="article-score-issue__content">
                <p class="article-score-issue__reason">{{ issue.reason }}</p>
                <p v-if="resolveIssueAnchor(issue)" class="article-score-issue__anchor">{{ resolveIssueAnchor(issue) }}</p>
                <p class="article-score-issue__suggestion">建议：{{ issue.suggestion }}</p>
              </div>
            </article>
          </div>
        </section>

        <section v-if="!hasDetails" class="article-score-section article-score-section--compact">
          <div class="article-score-section__title">维度摘要</div>
          <p class="article-score-section__comment">当前模式只展示公开维度分与等级，详细问题和综合建议仅作者可见。</p>
        </section>
      </div>

      <template #footer>
        <div class="article-score-footer">
          <NButton quaternary @click="close">关闭</NButton>
          <NButton v-if="allowRefresh" type="primary" :loading="refreshing" @click="emit('refresh')">
            {{ refreshLabel || "重新获取" }}
          </NButton>
        </div>
      </template>
    </NCard>
  </NModal>
</template>

<style scoped>
.article-score-modal {
  width: min(980px, calc(100vw - 32px));
}

.article-score-card {
  border-radius: 28px;
}

.article-score-loading,
.article-score-empty {
  min-height: 360px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 14px;
}

.article-score-loading {
  color: rgba(71, 85, 105, 0.92);
}

.article-score-loading__ring {
  width: 30px;
  height: 30px;
  border-radius: 999px;
  border: 3px solid rgba(15, 118, 110, 0.18);
  border-top-color: #0f766e;
  animation: article-score-spin 0.9s linear infinite;
}

.article-score-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.article-score-hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  padding: 20px 22px;
  border-radius: 24px;
  background: linear-gradient(135deg, rgba(240, 253, 250, 0.94), rgba(236, 253, 245, 0.72));
  border: 1px solid rgba(167, 243, 208, 0.72);
}

.article-score-hero__eyebrow {
  font-size: 12px;
  letter-spacing: 0.18em;
  color: #0f766e;
}

.article-score-hero__total {
  margin-top: 8px;
  font-size: clamp(40px, 7vw, 62px);
  line-height: 1;
  font-weight: 800;
  color: #0f172a;
}

.article-score-hero__meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 12px;
}

.article-score-hero__time {
  color: rgba(71, 85, 105, 0.88);
  font-size: 13px;
}

.article-score-hero__side {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 110px;
  padding: 14px 16px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.82);
  color: #1f2937;
}

.article-score-hero__side-label {
  font-size: 12px;
  color: rgba(71, 85, 105, 0.88);
}

.article-score-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.article-score-dimension {
  padding: 16px 18px;
  border-radius: 20px;
  border: 1px solid rgba(221, 228, 239, 0.92);
  background: rgba(248, 250, 252, 0.88);
}

.article-score-dimension__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
  color: #1f2937;
}

.article-score-dimension__reason {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.75;
  color: rgba(71, 85, 105, 0.92);
}

.article-score-section {
  padding: 20px 22px;
  border-radius: 24px;
  border: 1px solid rgba(221, 228, 239, 0.92);
  background: rgba(255, 255, 255, 0.92);
}

.article-score-section--compact {
  background: rgba(248, 250, 252, 0.88);
}

.article-score-section__title {
  font-size: 18px;
  font-weight: 700;
  color: #1f2937;
}

.article-score-section__comment {
  margin: 12px 0 0;
  line-height: 1.82;
  color: rgba(51, 65, 85, 0.95);
}

.article-score-issues {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.article-score-issue {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr);
  gap: 14px;
  padding: 16px 18px;
  border-radius: 20px;
  background: rgba(248, 250, 252, 0.86);
}

.article-score-issue__index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 14px;
  background: rgba(15, 118, 110, 0.12);
  color: #0f766e;
  font-weight: 700;
}

.article-score-issue__reason,
.article-score-issue__anchor,
.article-score-issue__suggestion {
  margin: 0;
  line-height: 1.78;
}

.article-score-issue__reason {
  color: #1f2937;
  font-weight: 600;
}

.article-score-issue__anchor {
  margin-top: 6px;
  color: rgba(100, 116, 139, 0.94);
  font-size: 13px;
}

.article-score-issue__suggestion {
  margin-top: 6px;
  color: rgba(51, 65, 85, 0.95);
}

.article-score-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@keyframes article-score-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 820px) {
  .article-score-grid {
    grid-template-columns: 1fr;
  }

  .article-score-hero {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
