<script setup lang="ts">
import { NButton, NList, NListItem, NThing, NTag, useMessage } from "naive-ui";
import { unfollowUser } from "~/services/follow";
import { getFollowUsers } from "~/services/studio";
import { formatDateTimeLabel } from "~/utils/format";
import { isMutualFollow } from "~/utils/relation";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const page = shallowRef(1);
const { data, pending, refresh } = await useAsyncData(
  () => `studio-follows:${page.value}`,
  () => getFollowUsers({ page: page.value, limit: 30 }),
  { watch: [page] },
);

async function removeFollow(id: string) {
  try {
    await unfollowUser(id);
    message.success("已取消关注");
    await refresh();
    if (!data.value?.list.length && page.value > 1) {
      page.value -= 1;
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : "取消关注失败");
  }
}

function previousPage() {
  if (page.value <= 1) return;
  page.value -= 1;
}

function nextPage() {
  if (!data.value?.has_more) return;
  page.value += 1;
}

useSeoMeta({
  title: "个人中心 - 关注",
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <NList v-if="data?.list.length">
        <NListItem v-for="item in data?.list" :key="item.followed_user_id">
          <NThing :title="item.followed_nickname" :description="item.followed_abstract || '这个用户还没有填写简介。'">
            <template #header-extra>
              <NTag size="small">{{ isMutualFollow(item.relation) ? "互相关注" : "已关注" }}</NTag>
            </template>
            <template #footer>
              <div class="studio-list-meta">
                <span>{{ formatDateTimeLabel(item.follow_time) }}</span>
              </div>
            </template>
          </NThing>
          <template #suffix>
            <div class="flex flex-wrap gap-2">
              <NuxtLink :to="`/users/${item.followed_user_id}`" class="glass-badge">查看主页</NuxtLink>
              <NButton quaternary size="small" @click="removeFollow(item.followed_user_id)">取消关注</NButton>
            </div>
          </template>
        </NListItem>
      </NList>
      <StudioEmptyState
        v-else
        title="你还没有关注任何人"
        :description="pending ? '正在读取关注列表…' : '可以从作者主页或文章详情作者卡开始关注感兴趣的创作者。'"
      />

      <div v-if="data?.list.length" class="mt-5 flex flex-wrap items-center justify-between gap-3 text-sm muted">
        <span>第 {{ page }} 页</span>
        <div class="flex items-center gap-3">
          <NButton quaternary size="small" :disabled="page <= 1 || pending" @click="previousPage()">上一页</NButton>
          <NButton quaternary size="small" :disabled="!data?.has_more || pending" @click="nextPage()">下一页</NButton>
        </div>
      </div>
    </section>
  </div>
</template>
