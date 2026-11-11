<script setup lang="ts">
/**
 * 个人中心 · 分类管理（票 327 建页，票 351 改版）。
 *
 * 版式从"一行一个分类 + 右侧操作"换成分类网格 + 大弹窗：一进来先看全部分类
 * （名称、篇数），点哪张就在弹窗里看该分类下的**全部**文章 —— 包括草稿，
 * 每篇带状态标签（票 351 故事 21）。作者的诉求是"在这个分类里管我的未发布稿"，
 * 只列已发布就做不到这件事。
 *
 * 写流程不在这里重写一遍：新建 / 改名 / 删除、二次确认（含受影响篇数）、失败原样
 * 展示、写完重拉列表都在 `useProfileCategoryManage` 里，它自己的用例不依赖这个页面。
 */
import { computed, ref, shallowRef, watch } from 'vue'
import { NModal, NSkeleton, useDialog, useMessage } from 'naive-ui'
import ArticleGridCard from '~/components/article/ArticleGridCard.vue'
import ProfileCategoryFormModal from '~/components/profile/ProfileCategoryFormModal.vue'
import ProfileEmptyState from '~/components/profile/ProfileEmptyState.vue'
import { useProfileCategoryManage } from '~/composables/useProfileCategoryManage'
import { getProfileCategories } from '~/services/category'
import { searchArticles } from '~/services/search'
import type { ProfileCategoryItem, SearchArticleItem } from '~/types/api'
import { errorMessage } from '~/utils/errorMessage'

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

const categories = computed<ProfileCategoryItem[]>(() => data.value?.list ?? []);
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

// ── 分类内容弹窗 ────────────────────────────────────────────────

/** 弹窗里一次取多少：2–3 列下正好几整排。 */
const PAGE_SIZE = 24;

const openCategoryId = shallowRef("");
const openCategory = computed(
  () => categories.value.find((item) => item.id === openCategoryId.value) ?? null,
);

const articles = ref<SearchArticleItem[]>([]);
const articlesLoading = ref(false);
const articlesError = ref("");

/**
 * 取该分类下的全部文章（含草稿）。
 *
 * 关键是**不带 status 过滤**：作者要在这里看到并管理未发布的稿子；
 * 公开主页那条读路径才是"只列已发布"。`type: 4` 是"我的文章"。
 */
async function loadArticles() {
  const categoryId = openCategoryId.value;
  if (!categoryId) return;
  articlesLoading.value = true;
  articlesError.value = "";
  try {
    const result = await searchArticles({
      type: 4,
      category_id: categoryId,
      page: 1,
      limit: PAGE_SIZE,
      sort: 2,
      page_mode: "count",
      auth: true,
    });
    articles.value = (result?.list ?? []) as unknown as SearchArticleItem[];
  } catch (error) {
    articlesError.value = errorMessage(error, "分类文章加载失败");
    articles.value = [];
  } finally {
    articlesLoading.value = false;
  }
}

function openCategoryModal(id: string) {
  openCategoryId.value = id;
  articles.value = [];
  void loadArticles();
}

function closeCategoryModal() {
  openCategoryId.value = "";
  articles.value = [];
  articlesError.value = "";
}

/** 状态标签：分类弹窗里每篇都要能看出"发没发出去"。 */
function statusLabel(status?: number) {
  switch (status) {
    case 1: return "草稿";
    case 2: return "待审核";
    case 3: return "已发布";
    case 4: return "已拒绝";
    default: return "";
  }
}

// 分类被删掉后弹窗要跟着关：否则会停在一个已不存在的分类上。
watch(categories, (list) => {
  if (openCategoryId.value && !list.some((item) => item.id === openCategoryId.value)) {
    closeCategoryModal();
  }
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

      <!-- 票 351 故事 14：分类也是一张张卡（名称 + 篇数），点开看里面的文章。 -->
      <div v-if="categories.length" class="mt-5 studio-folder-grid" data-role="category-grid">
        <article
          v-for="category in categories"
          :key="category.id"
          class="studio-folder-card"
          :data-role="`category-row-${category.id}`"
        >
          <button type="button" class="studio-folder-card__open" @click="openCategoryModal(category.id)">
            <header class="studio-folder-card__head">
              <strong class="studio-folder-card__title" :data-role="`category-title-${category.id}`">{{ category.title }}</strong>
              <span class="studio-folder-card__count" :data-role="`category-count-${category.id}`">
                {{ category.article_count ?? 0 }} 篇
              </span>
            </header>
            <p class="studio-folder-card__abstract">删除后该分类下的文章会变成「未分类」，文章本身不会删除。</p>
          </button>

          <div class="studio-folder-card__actions">
            <button
              type="button"
              class="profile-category-action"
              :data-role="`category-rename-${category.id}`"
              @click.stop="openRename(category)"
            >
              改名
            </button>
            <button
              type="button"
              class="profile-category-action is-danger"
              :data-role="`category-delete-${category.id}`"
              @click.stop="remove(category)"
            >
              删除
            </button>
          </div>
        </article>
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

    <!-- 分类内容：大弹窗 + 多列文章卡（含草稿，每篇带状态标签）。 -->
    <NModal
      :show="Boolean(openCategoryId)"
      preset="card"
      :bordered="false"
      style="width: min(1100px, calc(100vw - 32px)); max-height: 85vh"
      @update:show="(value: boolean) => { if (!value) closeCategoryModal() }">
      <template #header>
        <div class="favorite-modal__head">
          <h2 class="section-title">{{ openCategory?.title || '分类' }}</h2>
          <span class="muted">
            {{ articles.length }} 篇（含草稿） · 点卡片打开文章
          </span>
        </div>
      </template>

      <div v-if="openCategoryId" class="favorite-modal__body" data-role="category-articles">
        <div v-if="articles.length" class="article-grid">
          <div v-for="item in articles" :key="item.id" class="relative">
            <ArticleGridCard
              :title="item.title"
              :cover="item.cover"
              :date="item.updated_at || item.created_at"
              :to="`/article/${item.id}`"
              :view-count="item.view_count"
              :digg-count="item.digg_count"
              :comment-count="item.comment_count"
              :favor-count="item.favor_count" />
            <!-- 状态角标：草稿 / 待审核 / 已拒绝都要能一眼看出来（故事 21）。 -->
            <span
              v-if="statusLabel(item.publish_status)"
              class="category-status-badge"
              :data-status="item.publish_status"
              :data-role="`category-article-status-${item.id}`">
              {{ statusLabel(item.publish_status) }}
            </span>
          </div>
        </div>

        <div v-else-if="articlesLoading" class="article-grid">
          <article v-for="idx in 6" :key="`category-skeleton-${idx}`" class="article-grid-card">
            <NSkeleton class="article-grid-card__cover" />
            <NSkeleton text width="80%" class="mt-3" />
            <NSkeleton text width="55%" class="mt-2" />
          </article>
        </div>

        <ProfileEmptyState
          v-else
          size="lg"
          :title="articlesError ? '分类文章暂时打不开' : '这个分类下还没有文章'"
          :description="
            articlesError
              ? articlesError
              : '把文章归到这个分类后，它就会出现在这里（草稿也会显示）。'
          " />
      </div>
    </NModal>

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
