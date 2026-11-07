<script setup lang="ts">
import { computed } from "vue";
import { useMessage } from "naive-ui";

import InboxChatView from "~/components/inbox/InboxChatView.vue";
import InboxMessageList from "~/components/inbox/InboxMessageList.vue";
import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import { useMessageCenter } from "~/composables/useMessageCenter";
import { messageCenterNavItems, resolveMessageCenterTab } from "~/utils/messageCenter";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 消息中心顶级页（第 239 票起，2026-11-05 重构）。
 *
 * 路由边界：`ssr: false`（私人数据、不索引），由 `shared/readerRenderingBoundary.ts`
 * 的 `/messages/**` 派生；`auth` 中间件保证未登录先登录。
 *
 * 骨架：标题 + 顶部导航条 + 一张内容卡。五个子项共用同一张卡，
 * 顶栏 hover 面板消费同一份 `messageCenterNavItems`。私信子项
 * 才拉起会话取数、WS 订阅与草稿解析（拆成 InboxChatView 的理由）。
 *
 * 页头不再写"把五个子项聚合在这里"这类说明：导航本身已经说清了。
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
      return { title: "还没有点赞与收藏", description: "有人点赞或收藏你的内容时，会在这里提醒你。" };
    case "system":
      return { title: "还没有系统通知", description: "账号与站点的重要提醒会出现在这里。" };
    case "global":
      return { title: "还没有全局通知", description: "站点的全局公告会出现在这里。" };
    default:
      return { title: "还没有评论与回复", description: "有人评论或回复你的文章时，消息会出现在这里。" };
  }
});

async function handleMarkAllRead() {
  try {
    await markAllRead();
    message.success("已全部标记为已读");
  } catch (reason) {
    message.error(errorMessage(reason, "标记已读失败"));
  }
}

async function handleClearGroup() {
  try {
    await clearCurrentGroup();
    message.success("已清空当前分组");
  } catch (reason) {
    message.error(errorMessage(reason, "清空分组失败"));
  }
}

async function handleRemove(id: string) {
  try {
    await removeEntry(id);
    message.success("已删除这条消息");
  } catch (reason) {
    message.error(errorMessage(reason, "删除消息失败"));
  }
}

useSeoMeta({ title: "消息中心" });
useNoIndexPage();
</script>

<template>
  <div class="page-stack message-center">
    <header class="message-center__header">
      <h1 class="page-title">消息中心</h1>
    </header>

    <div class="message-center__layout">
      <!-- 顶部导航条：五个子项 + 未读角标。与顶栏 hover 面板同源。 -->
      <InboxSubNav :items="navItems" :active-tab="activeTab" />

      <section class="surface-card message-center__content">
        <!-- 私信子项：会话列表 + 聊天窗（宽屏双栏、<1024px 二选一）。 -->
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
      </section>
    </div>
  </div>
</template>
