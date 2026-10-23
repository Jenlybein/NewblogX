<script setup lang="ts">
import { NButton, NList, NListItem, NThing, NTag, useMessage } from "naive-ui";
import { followUser, unfollowUser } from "~/services/follow";
import { getFanUsers } from "~/services/studio";
import { formatDateTimeLabel } from "~/utils/format";
import { isFollowing, isMutualFollow } from "~/utils/relation";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const page = shallowRef(1);
const { data, pending, refresh } = await useAsyncData(
  () => `studio-fans:${page.value}`,
  () => getFanUsers({ page: page.value, limit: 30 }),
  { watch: [page] },
);

async function toggleFollow(id: string, relation: number) {
  try {
    if (isFollowing(relation)) {
      await unfollowUser(id);
      message.success("已取消回关");
    } else {
      await followUser(id);
      message.success("已回关");
    }
    await refresh();
    if (!data.value?.list.length && page.value > 1) {
      page.value -= 1;
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : "关注操作失败");
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
  title: "个人中心 - 粉丝",
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <NList v-if="data?.list.length">
        <NListItem v-for="item in data?.list" :key="item.fans_user_id">
          <NThing :title="item.fans_nickname" :description="item.fans_abstract || '这个用户还没有填写简介。'">
            <template #header-extra>
              <NTag size="small">{{ isMutualFollow(item.relation) ? "互相关注" : "关注了你" }}</NTag>
            </template>
            <template #footer>
              <div class="studio-list-meta">
                <span>{{ formatDateTimeLabel(item.follow_time) }}</span>
              </div>
            </template>
          </NThing>
          <template #suffix>
            <div class="flex flex-wrap gap-2">
              <NuxtLink :to="`/users/${item.fans_user_id}`" class="glass-badge">查看主页</NuxtLink>
              <NButton quaternary size="small" @click="toggleFollow(item.fans_user_id, item.relation)">
                {{ isMutualFollow(item.relation) ? "取消回关" : "回关" }}
              </NButton>
            </div>
          </template>
        </NListItem>
      </NList>
      <StudioEmptyState
        v-else
        title="你还没有粉丝"
        :description="pending ? '正在读取粉丝列表…' : '继续发布内容和参与互动后，这里会逐渐出现新的关系沉淀。'"
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
