<script setup lang="ts">
import { NButton, useMessage } from "naive-ui";
import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import { unfollowUser } from "~/services/follow";
import { getFollowUsers } from "~/services/studio";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 个人中心 - 关注（票 327）。
 *
 * 与粉丝页同一处修复：列表本体复用公开主页那份 `ProfileRelationList`，
 * 不再用 naive-ui `NList/NListItem/NThing` + `#suffix` 槽（`flex: 0` 会把中文操作按钮
 * 压成一字宽、竖排、把列表项撑高）。关注页是"我关注的人"，所以文案走关系语境
 * （默认 `mode`）：互相关注 → 「取消关注」，与公开主页保持一致。
 *
 * 分页仍留在页面。
 */
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
    message.error(errorMessage(error, "取消关注失败"));
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
useNoIndexPage();
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <ProfileRelationList
        title="关注"
        :items="data?.list ?? []"
        :pending="pending"
        @toggle-follow="removeFollow"
      />

      <div v-if="data?.list.length" class="mt-5 flex flex-wrap items-center justify-between gap-3 text-sm muted">
        <span>第 {{ page }} 页</span>
        <div class="flex items-center gap-3">
          <NButton
            quaternary
            size="small"
            data-role="pagination-prev"
            :disabled="page <= 1 || pending"
            @click="previousPage()"
          >
            上一页
          </NButton>
          <NButton
            quaternary
            size="small"
            data-role="pagination-next"
            :disabled="!data?.has_more || pending"
            @click="nextPage()"
          >
            下一页
          </NButton>
        </div>
      </div>
    </section>
  </div>
</template>
