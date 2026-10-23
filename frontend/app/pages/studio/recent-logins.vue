<script setup lang="ts">
import { NButton, NList, NListItem, NTag, NThing, useMessage } from "naive-ui";
import { getUserSessions, revokeUserSession } from "~/services/studio";
import { formatDateTimeLabel } from "~/utils/format";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const authStore = useAuthStore();
const message = useMessage();
const { data, pending, refresh } = await useAsyncData("studio-user-sessions", () => getUserSessions({ page: 1, limit: 20 }));

async function handleRevoke(id: string, isCurrent: boolean) {
  try {
    await revokeUserSession(id);
    message.success(isCurrent ? "当前设备已下线，即将清理本地登录状态" : "设备已下线");
    if (isCurrent) {
      authStore.clearSession();
      await navigateTo("/");
      return;
    }
    await refresh();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "下线设备失败");
  }
}

useSeoMeta({
  title: "个人中心 - 最近登录",
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <div class="studio-toolbar">
        <div class="muted text-sm">共 {{ data?.count ?? 0 }} 条登录记录</div>
        <button type="button" class="glass-badge" @click="refresh()">刷新记录</button>
      </div>

      <NList v-if="data?.list.length" class="mt-4">
        <NListItem v-for="item in data?.list" :key="item.id">
          <NThing :title="item.addr || item.ip" :description="item.ua">
            <template #header-extra>
              <div class="flex flex-wrap items-center gap-2">
                <NTag size="small">{{ item.ip }}</NTag>
                <NTag v-if="item.is_current" size="small" type="success">当前设备</NTag>
              </div>
            </template>
            <template #footer>
              <div class="studio-list-meta">
                <span>登录时间 {{ formatDateTimeLabel(item.created_at) }}</span>
                <span v-if="item.last_seen_at">最近活跃 {{ formatDateTimeLabel(item.last_seen_at) }}</span>
                <span>到期时间 {{ formatDateTimeLabel(item.expires_at) }}</span>
              </div>
            </template>
          </NThing>
          <template #suffix>
            <NButton
              quaternary
              size="small"
              @click="handleRevoke(item.id, item.is_current)"
            >
              {{ item.is_current ? "下线当前设备" : "下线设备" }}
            </NButton>
          </template>
        </NListItem>
      </NList>
      <StudioEmptyState
        v-else
        title="还没有登录记录"
        :description="pending ? '正在读取会话列表…' : '当前没有可管理的有效会话。'"
        class="mt-5"
      />
    </section>
  </div>
</template>
