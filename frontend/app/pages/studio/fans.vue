<script setup lang="ts">
import { NButton, useMessage } from "naive-ui";
import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import { followUser, unfollowUser } from "~/services/follow";
import { getFanUsers } from "~/services/studio";
import { isFollowing, isMutualFollow } from "~/utils/relation";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 个人中心 - 粉丝（票 327）。
 *
 * 列表本体复用公开主页那份 `ProfileRelationList`（头像 + 昵称 + 关系 chip + 简介 + 时间 + 操作）。
 *
 * 为什么不再自己拼 naive-ui 的 `NList / NListItem / NThing`：操作按钮放在
 * `NListItem` 的 `#suffix` 槽里，而 `.n-list-item__suffix` 是 `flex: 0`（basis 0、不 grow），
 * 这一列会退化成"内容的最小可断行宽度"；"查看主页"是中文、可任意断行，于是整列被压成
 * 一个字宽、四个字竖排、列表项被撑高。共享组件把操作放在自己的卡片栅格里，
 * 中文按钮不会被压扁（`.glass-badge` 另有 `white-space: nowrap` 兜底）。
 *
 * 分页仍留在页面：共享组件只管一页的展示，翻页由取数方决定。
 */
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
      // 粉丝语境：互相关注（4）撤掉的是"你回关的那一半"（提示也别写"取消关注"）；
      // 单向已关注（2）撤掉的就是关注本身。提示与按钮文案（见 utils/relation）一致。
      await unfollowUser(id);
      message.success(isMutualFollow(relation) ? "已取消回关" : "已取消关注");
    } else {
      await followUser(id);
      message.success("已回关");
    }
    // 写完以服务端为准：关系是双向的，本地改一条还会漏掉对方那侧的计数。
    await refresh();
    if (!data.value?.list.length && page.value > 1) {
      page.value -= 1;
    }
  } catch (error) {
    message.error(errorMessage(error, "关注操作失败"));
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
useNoIndexPage();
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <ProfileRelationList
        title="粉丝"
        mode="fans"
        :items="data?.list ?? []"
        :pending="pending"
        @toggle-follow="toggleFollow"
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
