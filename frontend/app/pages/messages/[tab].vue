<script setup lang="ts">
import { computed } from "vue";
import { useMessage } from "naive-ui";

import InboxChatView from "~/components/inbox/InboxChatView.vue";
import InboxMessageList from "~/components/inbox/InboxMessageList.vue";
import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import { useMessageCenter } from "~/composables/useMessageCenter";
import { messageCenterNavItems, resolveMessageCenterTab } from "~/utils/messageCenter";

/**
 * 消息中心顶级页（第 239 票起）：`/messages/{comments,digg,system,global,chat}`。
 *
 * 路由边界：`ssr: false`（私人数据、不索引），由 `shared/readerRenderingBoundary.ts`
 * 的 `/messages/**` 派生；`auth` 中间件保证未登录先登录。
 *
 * 本页只做"左子项导航 + 右内容"的骨架与容器：
 *   - 评论 / 互动 / 系统 / 全局走 `useMessageCenter` 的列表（第 239 票）；
 *   - 私信子项的右内容是「会话列表 + 聊天窗」，由 `InboxChatView` 复用既有的
 *     `InboxChatPanel` 与 `useInboxCenter`（第 240 票）。拆成子组件是为了**只有私信
 *     子项**才拉起会话取数、WS 订阅与草稿解析。
 */
definePageMeta({
  middleware: ["auth", "messages-tab"],
});

const route = useRoute();
const message = useMessage();
const messageStore = useMessageStore();

const activeTab = computed(() => resolveMessageCenterTab(route.params.tab));
const navItems = computed(() => messageCenterNavItems(messageStore.summary));

const { items, pending, hasMore, loadMore, markAllRead, clearCurrentGroup, removeEntry } = useMessageCenter({
  tab: activeTab,
});

const emptyCopy = computed(() => {
  switch (activeTab.value) {
    case "digg":
      return {
        title: "还没有点赞与收藏",
        description: "有人点赞或收藏你的内容时，会在这里提醒你。",
      };
    case "system":
      return {
        title: "还没有系统通知",
        description: "账号与站点的重要提醒会出现在这里。",
      };
    case "global":
      return {
        title: "还没有全局通知",
        description: "站点的全局公告会出现在这里。",
      };
    default:
      return {
        title: "还没有评论与回复",
        description: "有人评论或回复你的文章时，消息会出现在这里。",
      };
  }
});

async function handleMarkAllRead() {
  try {
    await markAllRead();
    message.success("已全部标记为已读");
  } catch (reason) {
    message.error(reason instanceof Error ? reason.message : "标记已读失败");
  }
}

async function handleClearGroup() {
  try {
    await clearCurrentGroup();
    message.success("已清空当前分组");
  } catch (reason) {
    message.error(reason instanceof Error ? reason.message : "清空分组失败");
  }
}

async function handleRemove(id: string) {
  try {
    await removeEntry(id);
    message.success("已删除这条消息");
  } catch (reason) {
    message.error(reason instanceof Error ? reason.message : "删除消息失败");
  }
}

useSeoMeta({
  title: "消息中心",
});
</script>

<template>
  <div class="page-stack message-center">
    <header>
      <h1 class="page-title">消息中心</h1>
      <p class="muted mt-2">评论、互动、系统与全局通知，以及私信会话都聚合在这里。</p>
    </header>

    <div class="message-center__layout">
      <InboxSubNav :items="navItems" :active-tab="activeTab" />

      <!-- 私信子项：右内容 = 会话列表 + 聊天窗（宽屏双栏、<1024px 二选一）。 -->
      <InboxChatView v-if="activeTab === 'chat'" />

      <InboxMessageList
        v-else
        :items="items"
        :pending="pending"
        :has-more="hasMore"
        :active-tab="activeTab"
        :empty-title="emptyCopy.title"
        :empty-description="emptyCopy.description"
        @mark-all-read="handleMarkAllRead()"
        @clear-group="handleClearGroup()"
        @remove="handleRemove($event)"
        @load-more="loadMore()"
      />
    </div>
  </div>
</template>
