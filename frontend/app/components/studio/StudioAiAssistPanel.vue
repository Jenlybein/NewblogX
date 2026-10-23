<script setup lang="ts">
import type { AiDiagnoseResponseData } from "~/types/api";
import type { StudioAiAction, StudioAiSelectionContext } from "~/composables/useStudioAiSelection";
import { computed } from "vue";
import { NButton, NCard, NInput, NModal, NScrollbar, NTag } from "naive-ui";

const props = defineProps<{
  show: boolean;
  action: StudioAiAction;
  selection: StudioAiSelectionContext | null;
  styleInstruction: string;
  overwriteResult: string;
  diagnoseResult: AiDiagnoseResponseData | null;
  pending: boolean;
  errorMessage: string;
  canSubmit: boolean;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
  "update:styleInstruction": [value: string];
  run: [];
  replace: [];
  "insert-below": [];
  copy: [];
}>();

const panelTitle = computed(() => {
  switch (props.action) {
    case "polish":
      return "AI 润色改写";
    case "grammar_fix":
      return "AI 语法纠错";
    case "style_transform":
      return "AI 风格转换";
    case "diagnose":
      return "AI 内容诊断";
    default:
      return "AI 辅助";
  }
});

const panelDescription = computed(() => {
  switch (props.action) {
    case "polish":
      return "保持原意，优化表达节奏、可读性和完成度。";
    case "grammar_fix":
      return "聚焦错别字、病句、标点和语法问题，不主动改写观点。";
    case "style_transform":
      return "根据目标风格重写选区内容，适合统一文章语气。";
    case "diagnose":
      return "输出结构化问题清单，不会直接改动正文。";
    default:
      return "";
  }
});

const primaryActionLabel = computed(() => {
  if (props.pending) {
    return props.action === "diagnose" ? "诊断中…" : "生成中…";
  }

  if (props.action === "diagnose") {
    return props.diagnoseResult ? "重新诊断" : "开始诊断";
  }

  return props.overwriteResult ? "重新生成" : "开始生成";
});

const selectedLength = computed(() => props.selection?.selectionText.trim().length || 0);

function closePanel() {
  emit("update:show", false);
}

function severityType(value: string): "error" | "warning" | "default" {
  if (value === "高") return "error";
  if (value === "中") return "warning";
  return "default";
}
</script>

<template>
  <NModal :show="show" class="studio-ai-modal" :mask-closable="!pending" @update:show="emit('update:show', $event)">
    <NCard :title="panelTitle" :bordered="false" closable class="studio-ai-panel" @close="closePanel">
      <div class="studio-ai-panel__header">
        <div>
          <p class="studio-ai-panel__desc">{{ panelDescription }}</p>
          <p v-if="selection" class="studio-ai-panel__meta">
            标题：{{ selection.articleTitle }} · 选区 {{ selectedLength }} 字 · 上下文 {{ selection.prefixText.length }}/{{ selection.suffixText.length }}
          </p>
        </div>
        <NTag size="small" type="info">{{ panelTitle }}</NTag>
      </div>

      <div v-if="action === 'style_transform'" class="studio-ai-panel__style">
        <label class="studio-ai-panel__field-label">目标风格</label>
        <NInput
          :value="styleInstruction"
          placeholder="例如：更专业克制、轻松口语化、技术博客风格、知乎答主风格"
          @update:value="emit('update:styleInstruction', $event)" />
      </div>

      <div v-if="errorMessage" class="studio-ai-panel__error">
        {{ errorMessage }}
      </div>

      <div class="studio-ai-panel__content">
        <section class="studio-ai-panel__section">
          <div class="studio-ai-panel__section-title">原文选区</div>
          <NScrollbar class="studio-ai-panel__scroll">
            <pre class="studio-ai-panel__pre">{{ selection?.selectionText || "当前没有可用选区。" }}</pre>
          </NScrollbar>
        </section>

        <section class="studio-ai-panel__section studio-ai-panel__section--result">
          <div class="studio-ai-panel__section-title">
            {{ action === "diagnose" ? "诊断结果" : "改写结果" }}
          </div>

          <template v-if="action === 'diagnose'">
            <div v-if="diagnoseResult" class="studio-ai-diagnose">
              <div class="studio-ai-diagnose__summary">
                <strong>摘要</strong>
                <p>{{ diagnoseResult.summary }}</p>
              </div>

              <div class="studio-ai-diagnose__issues">
                <article v-for="(issue, index) in diagnoseResult.issues" :key="`${issue.type}-${index}`" class="studio-ai-diagnose__issue">
                  <div class="studio-ai-diagnose__issue-top">
                    <NTag size="small">{{ issue.type }}</NTag>
                    <NTag size="small" :type="severityType(issue.severity)">{{ issue.severity }}</NTag>
                  </div>
                  <p><strong>问题：</strong>{{ issue.reason }}</p>
                  <p><strong>证据：</strong>{{ issue.evidence }}</p>
                  <p><strong>建议：</strong>{{ issue.suggestion }}</p>
                </article>
              </div>
            </div>
            <div v-else class="studio-ai-panel__empty">
              <span v-if="pending">AI 正在分析这段内容…</span>
              <span v-else>点击右下角按钮开始诊断。</span>
            </div>
          </template>

          <template v-else>
            <NScrollbar class="studio-ai-panel__scroll">
              <pre class="studio-ai-panel__pre">{{ overwriteResult || (pending ? "AI 正在生成改写结果…" : "点击右下角按钮开始生成。") }}</pre>
            </NScrollbar>
          </template>
        </section>
      </div>

      <template #footer>
        <div class="studio-ai-panel__footer">
          <div class="studio-ai-panel__footer-left">
            <NButton quaternary @click="closePanel">关闭</NButton>
            <NButton :disabled="!canSubmit" :loading="pending" type="primary" @click="emit('run')">
              {{ primaryActionLabel }}
            </NButton>
          </div>

          <div v-if="action !== 'diagnose'" class="studio-ai-panel__footer-right">
            <NButton quaternary :disabled="!overwriteResult" @click="emit('copy')">复制结果</NButton>
            <NButton quaternary :disabled="!overwriteResult" @click="emit('insert-below')">插入到下方</NButton>
            <NButton type="primary" :disabled="!overwriteResult" @click="emit('replace')">替换选区</NButton>
          </div>
        </div>
      </template>
    </NCard>
  </NModal>
</template>

<style scoped>
.studio-ai-modal {
  width: min(1040px, calc(100vw - 48px));
}

.studio-ai-panel {
  border-radius: 28px;
}

.studio-ai-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  margin-bottom: 16px;
}

.studio-ai-panel__desc,
.studio-ai-panel__meta {
  margin: 0;
}

.studio-ai-panel__desc {
  color: rgba(89, 100, 122, 0.92);
  line-height: 1.7;
}

.studio-ai-panel__meta {
  margin-top: 6px;
  color: rgba(110, 122, 142, 0.82);
  font-size: 13px;
}

.studio-ai-panel__style {
  margin-bottom: 16px;
}

.studio-ai-panel__field-label {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: rgba(44, 58, 84, 0.92);
}

.studio-ai-panel__error {
  margin-bottom: 16px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(255, 96, 96, 0.08);
  color: #b03c4b;
  line-height: 1.7;
}

.studio-ai-panel__content {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.18fr);
  gap: 16px;
}

.studio-ai-panel__section {
  min-height: 360px;
  border: 1px solid rgba(215, 223, 235, 0.92);
  border-radius: 22px;
  background: rgba(248, 251, 255, 0.72);
  overflow: hidden;
}

.studio-ai-panel__section-title {
  padding: 14px 18px;
  border-bottom: 1px solid rgba(215, 223, 235, 0.92);
  font-weight: 700;
  color: rgba(33, 44, 65, 0.94);
}

.studio-ai-panel__scroll {
  max-height: 430px;
}

.studio-ai-panel__pre {
  margin: 0;
  padding: 18px;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.78;
  color: rgba(38, 48, 68, 0.95);
  font-family: "Maple Mono", "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 13.5px;
}

.studio-ai-panel__empty {
  min-height: 300px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  color: rgba(110, 122, 142, 0.86);
}

.studio-ai-diagnose {
  padding: 18px;
}

.studio-ai-diagnose__summary p,
.studio-ai-diagnose__issue p {
  margin: 8px 0 0;
  line-height: 1.72;
  color: rgba(46, 56, 75, 0.95);
}

.studio-ai-diagnose__issues {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.studio-ai-diagnose__issue {
  padding: 14px 16px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(220, 228, 240, 0.9);
}

.studio-ai-diagnose__issue-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.studio-ai-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.studio-ai-panel__footer-left,
.studio-ai-panel__footer-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

@media (max-width: 960px) {
  .studio-ai-modal {
    width: min(100vw, calc(100vw - 20px));
  }

  .studio-ai-panel__content {
    grid-template-columns: 1fr;
  }

  .studio-ai-panel__section {
    min-height: 280px;
  }
}
</style>
