<script setup lang="ts">
import { computed } from "vue";
import ArticleGridCard from "~/components/article/ArticleGridCard.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { ProfileCategoryItem, SearchArticleItem } from "~/types/api";

/**
 * 个人主页的「分类」栏（票 321）：左边列这位作者的分类，点一个就在右边就地展开
 * 该作者该分类下的文章，不跳页。
 *
 * 本组件**只读**：不判断身份、不发写请求，也没有任何写入口插槽（票 327 口径）。
 * 分类的集中管理（新建 / 改名 / 删除）只有一个地方 —— 个人中心 `/studio/categories`，
 * 复用 `useProfileCategoryManage` 与 `ProfileCategoryFormModal`。公开主页对任何身份
 * （包括作者本人）都只展示：分类列表、文章数、点分类展开文章、空态与失败态。
 *
 * 数据（分类列表、分类下的文章）都由页面取好传进来，选中的分类通过
 * `update:activeCategoryId` 回传，页面据此再取文章。
 */
const props = withDefaults(
  defineProps<{
    categories: ProfileCategoryItem[];
    activeCategoryId?: string;
    articles?: SearchArticleItem[];
    /** 分类列表加载中。 */
    pending?: boolean;
    /** 选中分类下的文章加载中。 */
    articlesPending?: boolean;
    /** 选中分类下的文章读取失败。 */
    articlesFailed?: boolean;
    /** 分类列表读取失败。 */
    failed?: boolean;
  }>(),
  {
    activeCategoryId: "",
    articles: () => [],
    pending: false,
    articlesPending: false,
    articlesFailed: false,
    failed: false,
  },
);

const emit = defineEmits<{
  "update:activeCategoryId": [value: string];
}>();

const activeCategory = computed(
  () => props.categories.find((category) => category.id === props.activeCategoryId) || null,
);

/** 点当前分类 = 收起；点别的分类 = 换成它。选中态始终只有一个。 */
function toggleCategory(id: string) {
  emit("update:activeCategoryId", props.activeCategoryId === id ? "" : id);
}
</script>

<template>
  <div class="profile-category-layout">
    <aside class="profile-category-layout__folders" aria-label="作者分类">
      <ProfileEmptyState
        v-if="failed"
        size="sm"
        title="分类暂时打不开"
        description="分类列表这次没有读到，稍后刷新重试；文章流不受影响。"
      />
      <ProfileEmptyState
        v-else-if="pending && !categories.length"
        size="sm"
        pending
        title="正在读取分类"
        description="分类列表正在读取，稍后会自动出现。"
      />
      <ProfileEmptyState
        v-else-if="!categories.length"
        size="sm"
        title="这位作者还没有分类"
        description="作者还没有把文章归到分类里，可以先看文章流。"
      />

      <template v-else>
        <!-- 票 351 故事 22 / 23：宽屏是 180px 窄栏，窄屏降级成一行可横滑的 chip。
             两种形态都是"一行一个分类 + 篇数"，不再有 72px 高的大卡。 -->
        <button
          v-for="category in categories"
          :key="category.id"
          type="button"
          class="profile-category-card"
          :class="{ 'is-active': activeCategoryId === category.id }"
          :aria-pressed="activeCategoryId === category.id"
          :aria-label="`${category.title}：${category.article_count} 篇公开文章`"
          :data-role="`category-chip-${category.id}`"
          @click="toggleCategory(category.id)"
        >
          <strong class="profile-category-card__title">{{ category.title }}</strong>
          <span class="profile-category-card__count">{{ category.article_count }}</span>
        </button>
      </template>
    </aside>

    <section class="profile-category-layout__content" aria-label="分类下的文章">
      <ProfileEmptyState
        v-if="!activeCategory"
        size="lg"
        :title="categories.length ? '选一个分类查看文章' : '还没有可以展开的分类'"
        :description="
          categories.length
            ? '点左边的任意一个分类，这位作者在该分类下的公开文章会直接出现在这里。'
            : '这位作者还没有公开分类，可以回到文章流或收藏栏继续浏览。'
        "
        :hints="categories.length ? ['展开只读取公开文章，草稿与私密文章不会出现。'] : []"
      />

      <ProfileEmptyState
        v-else-if="articlesPending && !articles.length"
        size="lg"
        pending
        title="正在读取分类文章"
        description="这个分类下的文章正在读取，稍后会自动出现。"
      />

      <ProfileEmptyState
        v-else-if="articlesFailed"
        size="lg"
        title="分类文章暂时打不开"
        description="这个分类下的文章这次没有读到，稍后刷新重试；分类列表本身是好的。"
      />

      <div v-else-if="articles.length" class="profile-category-article-list" data-role="category-articles">
        <!-- 不设顶栏：左边选中的分类已经高亮，右栏再来一条「Vue3 · 4 篇」
             只是把同一个信息说第二遍，还白占一行高度（2026-11-12）。 -->
        <!-- 多列文章卡（票 345 抽出来的共享卡）：封面 + 标题 + 日期 + 四个计数。
             窄栏旁边一列能放下 2–3 张，扫起来比原来的单列条目快得多。 -->
        <div class="profile-article-grid">
          <ArticleGridCard
            v-for="item in articles"
            :key="item.id"
            :title="item.title"
            :cover="item.cover"
            :date="item.updated_at || item.created_at"
            :to="`/article/${item.id}`"
            :view-count="item.view_count"
            :digg-count="item.digg_count"
            :comment-count="item.comment_count"
            :favor-count="item.favor_count" />
        </div>
      </div>

      <ProfileEmptyState
        v-else
        size="lg"
        title="这个分类下还没有公开文章"
        :description="`「${activeCategory.title}」下暂时没有公开文章，换一个分类继续看看。`"
        :hints="['分类里只有公开文章会出现在这里。']"
      />
    </section>
  </div>
</template>
