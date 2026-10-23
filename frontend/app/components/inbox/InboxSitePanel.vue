<script setup lang="ts">
import { nextTick, useTemplateRef, watch } from "vue";
import { NButton, NList, NListItem, NSpace, NTag, NThing } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { SiteMessageItem } from "~/types/api";
import type { SiteMessageGroup } from "~/composables/useInboxCenter";
import { formatDateTimeLabel } from "~/utils/format";

type InboxMessageGroup = SiteMessageGroup | "global";

const props = defineProps<{
  categories: Array<{ key: InboxMessageGroup; label: string; hint: string; count: number }>;
  activeGroup: InboxMessageGroup;
  items: SiteMessageItem[];
  pending: boolean;
  hasMore: boolean;
}>();

const emit = defineEmits<{
  "update:activeGroup": [value: InboxMessageGroup];
  markAllRead: [];
  remove: [id: string];
  clearGroup: [];
  loadMore: [];
}>();

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

function typeLabel(type: number) {
  if (type === 1 || type === 2) return "评论";
  if (type >= 3 && type <= 8) return "互动";
  return "系统";
}
</script>

<template>
  <section class="surface-card studio-inbox-card">
    <div class="studio-inbox-grid">
      <aside class="studio-inbox-grid__aside">
        <button
          v-for="item in categories"
          :key="item.key"
          type="button"
          class="studio-filter-chip studio-filter-chip--stack"
          :class="{ 'is-active': activeGroup === item.key }"
          @click="emit('update:activeGroup', item.key)"
        >
          <span class="studio-filter-chip__main">
            <strong>{{ item.label }}</strong>
            <small>{{ item.hint }}</small>
          </span>
          <span class="studio-sidebar__badge">{{ item.count }}</span>
        </button>
      </aside>

      <div class="studio-inbox-grid__main">
        <div class="studio-toolbar">
          <div>
            <h2 class="section-title">站内消息</h2>
            <p class="muted">评论、点赞收藏和系统提醒都会聚合在这里。</p>
          </div>
          <NSpace>
            <NButton quaternary @click="emit('markAllRead')">全部已读</NButton>
            <NButton quaternary @click="emit('clearGroup')">清空当前分组</NButton>
          </NSpace>
        </div>

        <div
          v-if="items.length"
          ref="listScroller"
          class="space-y-3 max-h-[640px] overflow-y-auto pr-1"
          @scroll.passive="tryLoadMore()">
          <NList>
            <NListItem v-for="item in items" :key="item.id">
              <NThing :title="item.link_title || item.article_title" :description="item.content">
                <template #avatar>
                  <AppAvatar :src="item.action_user_avatar" :name="item.action_user_nickname" fallback="系" />
                </template>
                <template #header-extra>
                  <NSpace size="small">
                    <NTag size="small">{{ typeLabel(item.type) }}</NTag>
                    <NTag v-if="!item.is_read" size="small" type="warning">未读</NTag>
                  </NSpace>
                </template>
                <template #footer>
                  <div class="studio-list-meta">
                    <span>{{ formatDateTimeLabel(item.created_at) }}</span>
                    <NuxtLink v-if="item.link_herf" :to="item.link_herf" class="glass-badge">查看原文</NuxtLink>
                  </div>
                </template>
              </NThing>
              <template #suffix>
                <NButton quaternary size="small" @click="emit('remove', item.id)">删除</NButton>
              </template>
            </NListItem>
          </NList>

          <div class="mt-3 flex items-center justify-center text-sm muted">
            <span v-if="pending">正在加载更多消息…</span>
            <span v-else-if="hasMore">继续下滑以加载更多</span>
            <span v-else>已经到底了</span>
          </div>
        </div>

        <StudioEmptyState
          v-else
          title="当前分组还没有消息"
          :description="pending ? '正在同步消息数据…' : '你的消息收件箱很安静，等互动出现后这里会自动聚合。'"
        />
      </div>
    </div>
  </section>
</template>
