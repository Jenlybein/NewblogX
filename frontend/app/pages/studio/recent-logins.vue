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
  title: "个人中心 - 登录设备",
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <div class="studio-toolbar">
        <div class="muted text-sm">共 {{ data?.count ?? 0 }} 台登录设备</div>
        <button type="button" class="glass-badge" @click="refresh()">刷新设备</button>
      </div>

      <NList v-if="data?.list.length" class="mt-4">
        <NListItem v-for="item in data?.list" :key="item.id">
          <NThing :title="item.device" :description="item.addr || item.ip">
            <template #header-extra>
              <div class="flex flex-wrap items-center gap-2">
                <NTag size="small">{{ item.ip }}</NTag>
                <NTag v-if="item.device_type === 'mobile'" size="small">手机</NTag>
                <NTag v-else-if="item.device_type === 'tablet'" size="small">平板</NTag>
                <NTag v-if="item.is_current" size="small" type="success">当前设备</NTag>
              </div>
            </template>
            <template #footer>
              <div class="studio-list-meta">
                <span>首次登录 {{ formatDateTimeLabel(item.first_seen_at) }}</span>
                <span v-if="item.last_seen_at">最近活跃 {{ formatDateTimeLabel(item.last_seen_at) }}</span>
                <span>累计登录 {{ item.login_count }} 次</span>
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
        title="还没有登录设备"
        :description="pending ? '正在读取登录设备…' : '当前没有可管理的登录设备。'"
        class="mt-5"
      />
    </section>
  </div>
</template>
