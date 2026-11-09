<script setup lang="ts">
import { computed } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ProfileCategoryFormModal from "~/components/profile/ProfileCategoryFormModal.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useProfileCategoryManage } from "~/composables/useProfileCategoryManage";
import { getProfileCategories } from "~/services/category";

/**
 * 个人中心 - 分类管理（票 327）。
 *
 * 分类的管理只属于个人中心：公开主页 `/users/[id]` 的分类栏是**只读**的（对任何身份
 * 都只展示列表、文章数、点分类展开文章），写入口一律在本页 —— 在写作页或读者页里
 * 翻自己的分类列表才能改名，定位是错的。
 *
 * 写流程不在这里重写一遍：新建 / 改名 / 删除、二次确认（含受影响篇数）、失败原样
 * 展示、写完重拉列表都在 `useProfileCategoryManage` 里，它自己的用例不依赖这个页面。
 * 这页只管三件事：列出来（标题 + 文章数）、把按钮接上、空态 / 加载态。
 *
 * 删除不可逆且会影响文章归属，所以确认弹窗交 naive-ui 的 dialog：
 * "该分类下有 N 篇文章，删除后它们会变成未分类，文章本身不会删除"。
 */
definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const authStore = useAuthStore();
const message = useMessage();
const dialog = useDialog();

const { data, pending, refresh } = await useAsyncData(
  () => `studio-categories:${authStore.profileId ?? "guest"}`,
  () =>
    authStore.profileId
      ? getProfileCategories(authStore.profileId)
      : Promise.resolve({ list: [], count: 0 }),
  // profileId 由登录态派生：晚到一次也要重取，否则会停在空列表。
  { watch: [computed(() => authStore.profileId)] },
);

const categories = computed(() => data.value?.list ?? []);
const articleTotal = computed(() =>
  categories.value.reduce((sum, category) => sum + (category.article_count ?? 0), 0),
);

const {
  formOpen,
  formTitle,
  formMode,
  saving,
  openCreate,
  openRename,
  submit,
  remove,
} = useProfileCategoryManage({
  message,
  // 二次确认：删除不可逆，先讲清楚会影响几篇文章、文章本身不会删。
  confirm: ({ title, content, onConfirm }) =>
    dialog.warning({
      title,
      content,
      positiveText: "删除",
      negativeText: "取消",
      onPositiveClick: onConfirm,
    }),
  // 写完以服务端为准（新建可能是恢复软删的那一条，本地拼条目会猜错）。
  reload: () => refresh(),
});

useSeoMeta({
  title: "个人中心 - 分类管理",
});
useNoIndexPage();
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <div class="studio-toolbar">
        <div class="min-w-0">
          <div class="eyebrow">Taxonomy</div>
          <h1 class="section-title mt-1">分类管理</h1>
          <p class="muted mt-1 text-sm">
            共 {{ categories.length }} 个分类<template v-if="categories.length">，汇总 {{ articleTotal }} 篇公开文章</template>
          </p>
        </div>
        <button
          type="button"
          class="profile-category-action is-primary"
          data-role="category-create"
          @click="openCreate"
        >
          新建分类
        </button>
      </div>

      <div v-if="categories.length" class="mt-5 flex flex-col gap-3" data-role="category-manage-list">
        <div
          v-for="category in categories"
          :key="category.id"
          class="flex flex-wrap items-center justify-between gap-3 rounded-card border border-line bg-surface p-4"
          :data-role="`category-row-${category.id}`"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <strong
                class="truncate text-base font-semibold text-ink"
                :data-role="`category-title-${category.id}`"
              >{{ category.title }}</strong>
              <span class="blueprint-chip" :data-role="`category-count-${category.id}`">
                {{ category.article_count ?? 0 }} 篇文章
              </span>
            </div>
            <p class="muted mt-1 text-xs">删除后该分类下的文章会变成「未分类」，文章本身不会删除。</p>
          </div>

          <div class="profile-category-card__actions">
            <button
              type="button"
              class="profile-category-action"
              :data-role="`category-rename-${category.id}`"
              @click="openRename(category)"
            >
              改名
            </button>
            <button
              type="button"
              class="profile-category-action is-danger"
              :data-role="`category-delete-${category.id}`"
              @click="remove(category)"
            >
              删除
            </button>
          </div>
        </div>
      </div>

      <ProfileEmptyState
        v-else
        class="mt-5"
        :pending="pending"
        :title="pending ? '正在读取分类' : '还没有分类'"
        :description="
          pending
            ? '正在从服务端同步你的分类列表，稍候会自动出现。'
            : '新建第一个分类，把文章按主题归档；读者在公开主页的分类栏里也能按分类浏览。'
        "
        :hints="pending ? [] : ['分类名不能为空，同名分类会被拒绝（删过的名字可以再用）。']"
      />
    </section>

    <!-- 新建 / 改名共用一个弹窗：标题与回填由 mode 决定，输入与校验在 composable 里。 -->
    <ProfileCategoryFormModal
      v-model:show="formOpen"
      :mode="formMode"
      :initial-title="formMode === 'rename' ? formTitle : ''"
      :pending="saving"
      @submit="submit"
    />
  </div>
</template>
