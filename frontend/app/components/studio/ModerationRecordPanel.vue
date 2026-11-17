<script setup lang="ts">
/**
 * 我的违规记录（ADR-0035）。
 *
 * 用户能看到的只有三件事：当前有效分、档位、以及每条记录能不能申诉。
 * 这里刻意**不显示命中词** —— 把"该绕开哪些词"告诉用户等于把规则送出去
 * （与 ADR-0032 决策 3 同一口径）。
 *
 * 分数展示保留一位小数：衰减是连续函数，取整会让用户对不上账
 * （"我明明只有几条记录，怎么分数是整数"）。
 */
import { computed, ref } from "vue";
import { NAlert, NButton, NEmpty, NInput, NTag, useMessage } from "naive-ui";
import { getMyViolations, submitViolationAppeal } from "~/services/moderation";
import type { ModerationEventItem } from "~/types/api";
import { errorMessage } from "~/utils/errorMessage";

const message = useMessage();

const { data, refresh, pending } = await useAsyncData("studio-moderation-me", () => getMyViolations());
const events = computed<ModerationEventItem[]>(() => data.value?.items ?? []);
const score = computed(() => Number(data.value?.score ?? 0).toFixed(1));
const hasRecord = computed(() => events.value.length > 0);

/** 申诉输入框：按事件 id 分开存，避免"给 A 填的理由跑到 B 上"。 */
const reasons = ref<Record<string, string>>({});
const submitting = ref("");

function reasonOf(id: string) {
  return reasons.value[id] ?? "";
}

function setReason(id: string, value: string) {
  reasons.value = { ...reasons.value, [id]: value };
}

async function appeal(event: ModerationEventItem) {
  const id = String(event.id);
  const reason = reasonOf(id).trim();
  if (!reason) {
    message.warning("请填写申诉理由");
    return;
  }
  submitting.value = id;
  try {
    await submitViolationAppeal({ event_id: event.id, reason });
    message.success("申诉已提交，处理结果会以站内信通知");
    setReason(id, "");
    await refresh();
  } catch (error) {
    message.error(errorMessage(error, "提交申诉失败"));
  } finally {
    submitting.value = "";
  }
}

/** 档位标签配色：处罚越重越醒目。 */
function tagType(punishment: string) {
  if (punishment === "none") return "success";
  if (punishment === "manual_review") return "error";
  return "warning";
}

function formatTime(value: string) {
  if (!value) return "-";
  return value.replace("T", " ").slice(0, 16);
}
</script>

<template>
  <div class="moderation-panel" :aria-busy="pending">
    <NAlert v-if="!hasRecord" type="success" :bordered="false">
      没有违规记录。当前风险分 {{ score }}。
    </NAlert>

    <template v-else>
      <div class="moderation-panel__head">
        <span class="moderation-panel__label">当前风险分</span>
        <strong class="moderation-panel__score">{{ score }}</strong>
        <NTag :type="tagType(String(data?.punishment || 'none'))" size="small">
          {{ data?.punishment_label || "仅记录，不处罚" }}
        </NTag>
      </div>
      <p class="moderation-panel__hint">
        近 90 天的违规按 60 天半衰期衰减；累计到 100 / 120 / 150 分依次禁言 1 / 7 / 30 天，200 分转人工评估。
        每条处罚都可以在 7 天内申诉一次。
      </p>

      <ul class="moderation-panel__list">
        <li v-for="event in events" :key="String(event.id)" class="moderation-panel__item">
          <div class="moderation-panel__row">
            <span class="moderation-panel__evidence">{{ event.evidence }}</span>
            <span class="moderation-panel__meta">
              +{{ event.score }} 分 · {{ formatTime(String(event.created_at)) }}
            </span>
          </div>
          <div class="moderation-panel__row">
            <NTag size="small" :type="event.status === 'revoked' ? 'success' : 'default'">
              {{ event.status === "revoked" ? "已撤销" : "生效中" }}
            </NTag>
            <div v-if="event.can_appeal && event.status === 'active'" class="moderation-panel__appeal">
              <NInput
                :value="reasonOf(String(event.id))"
                size="small"
                placeholder="说明情况后提交申诉"
                :maxlength="512"
                @update:value="(value: string) => setReason(String(event.id), value)" />
              <NButton
                size="small"
                secondary
                :loading="submitting === String(event.id)"
                @click="appeal(event)">
                申诉
              </NButton>
            </div>
          </div>
        </li>
      </ul>
    </template>

    <NEmpty v-if="hasRecord && events.length === 0" description="暂无记录" />
  </div>
</template>

<style scoped>
/*
 * 记录面板（ADR-0035）：一列时间线，分数在最上面。
 *
 * 只用内容色 token，不引入新的语义色：处罚档位靠 NTag 的类型表达，
 * 这里再上色会让「我的账号设置」页出现与全站不一致的红黄绿。
 */
.moderation-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.moderation-panel__head {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
}

.moderation-panel__label {
  font-size: 0.875rem;
  color: var(--color-text-secondary);
}

.moderation-panel__score {
  font-size: 1.5rem;
  font-variant-numeric: tabular-nums;
}

.moderation-panel__hint {
  margin: 0;
  font-size: 0.8125rem;
  line-height: 1.6;
  color: var(--color-text-secondary);
}

.moderation-panel__list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.moderation-panel__item {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  padding: 0.75rem;
  border: 1px solid var(--color-border);
  /* 圆角只取 token（票 204 的五档口径），不写字面量。 */
  border-radius: var(--radius-md);
}

.moderation-panel__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.moderation-panel__evidence {
  font-size: 0.875rem;
}

.moderation-panel__meta {
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
}

.moderation-panel__appeal {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
}
</style>

