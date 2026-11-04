<script setup lang="ts">
import { nextTick, ref, useTemplateRef, watch } from "vue";
import { IconTrash } from "@tabler/icons-vue";
import { NButton } from "naive-ui";

import StudioEmptyState from "~/components/studio/StudioEmptyState.vue";
import type { MessageCenterEntry, MessageCenterTab } from "~/utils/messageCenter";
import { formatDateTimeLabel } from "~/utils/format";

/**
 * 消息中心列表面板（第 239 票）。
 *
 * 列表行只留三样事实：文章标题（可点回原文）+ 一行内容摘要 + 时间；未读用
 * 左侧强调条 + 标题加粗 + 右上小圆点表达（不用 chip）。删除按钮默认 opacity:0，
 * hover / focus 才出现（CSS 在 tailwind.css）。
 *
 * 批量动作刻意只有三个：全部已读、清空当前分组（二次确认）、单条删除。
 * **不做多选**——消息中心的使用场景是"扫一眼清掉"，多选态没有真实需求。
 */
const props = defineProps<{
  items: MessageCenterEntry[];
  pending: boolean;
  hasMore: boolean;
  activeTab: MessageCenterTab;
  emptyTitle: string;
  emptyDescription: string;
}>();

const emit = defineEmits<{
  markAllRead: [];
  clearGroup: [];
  remove: [id: string];
  loadMore: [];
}>();

// "清空当前分组"是不可逆操作：第一次点击只展开确认条，第二次点击才真正执行。
const confirmingClear = ref(false);

watch(
  () => props.activeTab,
  () => {
    confirmingClear.value = false;
  },
);

function requestClearGroup() {
  if (!confirmingClear.value) {
    confirmingClear.value = true;
    return;
  }
  confirmingClear.value = false;
  emit("clearGroup");
}

function cancelClear() {
  confirmingClear.value = false;
}

const listScrollerRef = useTemplateRef<HTMLDivElement>("listScroller");

function tryLoadMore() {
  const scroller = listScrollerRef.value;
  if (!scroller || props.pending || !props.hasMore) {
    return;
  }
  const distanceToBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
  if (distanceToBottom <= 80) {
    emit("loadMore");
  }
}

// 列表首屏不够高时也要继续拉：把"要不要加载更多"的判断放在组件里，
// 页面只处理 `loadMore` 事件，滚动容器换实现时不用改页面。
watch(
  () => listScrollerRef.value,
  async (scroller) => {
    if (!scroller) {
      return;
    }
    await nextTick();
    tryLoadMore();
  },
);

watch(
  () => [props.items.length, props.hasMore, props.pending],
  async () => {
    await nextTick();
    tryLoadMore();
  },
);
</script>

<template>
  <section class="message-center__main">
    <div class="message-center-toolbar">
      <div>
        <h2 class="section-title">消息</h2>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <NButton quaternary size="small" @click="emit('markAllRead')">全部已读</NButton>
        <NButton quaternary size="small" @click="requestClearGroup()">清空当前分组</NButton>
      </div>
    </div>

    <div v-if="confirmingClear" class="message-center-confirm" role="alertdialog" aria-label="确认清空当前分组">
      <span>清空后不可恢复，确定继续？</span>
      <div class="flex flex-wrap items-center gap-2">
        <NButton size="small" @click="cancelClear()">取消</NButton>
        <NButton size="small" type="error" @click="requestClearGroup()">确认清空</NButton>
      </div>
    </div>

    <div
      v-if="items.length"
      ref="listScroller"
      class="message-list message-list__scroller"
      @scroll.passive="tryLoadMore()"
    >
      <article v-for="item in items" :key="item.id" class="message-row" :class="{ 'is-unread': item.unread }">
        <span v-if="item.unread" class="message-row__dot" aria-hidden="true" />

        <div class="message-row__body">
          <NuxtLink v-if="item.href" :to="item.href" class="message-row__title">{{ item.title }}</NuxtLink>
          <span v-else class="message-row__title">{{ item.title }}</span>
          <p class="message-row__summary">{{ item.summary }}</p>
          <time class="message-row__time">{{ formatDateTimeLabel(item.createdAt) }}</time>
        </div>

        <button
          type="button"
          class="message-row__delete"
          aria-label="删除这条消息"
          @click="emit('remove', item.id)"
        >
          <IconTrash :size="14" aria-hidden="true" />
        </button>
      </article>

      <div class="message-list__footer">
        <span v-if="pending">正在加载更多消息…</span>
        <span v-else-if="hasMore">继续下滑加载更多</span>
        <span v-else>没有更多了</span>
      </div>
    </div>

    <StudioEmptyState
      v-else
      :title="emptyTitle"
      :description="pending ? '正在同步消息数据…' : emptyDescription"
    />
  </section>
</template>
