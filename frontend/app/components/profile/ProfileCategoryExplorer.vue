<script setup lang="ts">
import { computed } from "vue";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { ProfileCategoryItem, SearchArticleItem } from "~/types/api";

/**
 * 个人主页的「分类」栏（票 321）：左边列这位作者的分类，点一个就在右边就地展开
 * 该作者该分类下的文章，不跳页。
 *
 * 本组件**只读**：数据（分类列表、分类下的文章）都由页面取好传进来，选中的分类
 * 通过 `update:activeCategoryId` 回传，页面据此再取文章。匿名访客与他人访问走的是
 * 同一条路径，所以这里不渲染任何写入口。
 *
 * 扩展位（留给下游的写操作票）：`toolbar`（分栏级操作，比如"新建分类"）与
 * `category-actions`（单个分类的行内操作）。两个插槽默认为空，只读视图不受影响；
 * 是否公开由页面按 `isSelf` 决定，组件本身不判断身份。
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
      <div v-if="$slots.toolbar" class="profile-category-toolbar">
        <slot name="toolbar" />
      </div>

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
        <div v-for="category in categories" :key="category.id" class="profile-category-card-wrap">
          <button
            type="button"
            class="profile-category-card"
            :class="{ 'is-active': activeCategoryId === category.id }"
            :aria-pressed="activeCategoryId === category.id"
            @click="toggleCategory(category.id)"
          >
            <span class="profile-category-card__title-row">
              <strong class="profile-category-card__title">{{ category.title }}</strong>
              <span class="blueprint-chip">{{ category.article_count }}</span>
            </span>
          </button>
          <slot name="category-actions" :category="category" />
        </div>
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

      <div v-else-if="articles.length" class="profile-category-article-list">
        <div class="profile-category-heading">
          <span class="profile-category-heading__title">{{ activeCategory.title }}</span>
          <span class="blueprint-chip">{{ articles.length }} 篇</span>
        </div>
        <ArticleFeedItem
          v-for="item in articles"
          :key="item.id"
          :article="item"
          variant="shelf"
          :show-author="false"
        />
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
