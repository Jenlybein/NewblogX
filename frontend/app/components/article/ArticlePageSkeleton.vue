<script setup lang="ts">
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";

const props = withDefaults(
  defineProps<{
    /** 文章有归属树时，骨架也占住左栏（与真实三栏版式同构，树到达后不跳版）。 */
    withOutline?: boolean;
  }>(),
  { withOutline: false },
);
</script>

<template>
  <div class="article-skeleton" data-role="article-page-skeleton" data-state="pending">
    <!--
      文章页切页骨架（ADR-0019 决策 3）：客户端跳转时 setup 不再等正文，
      页面先按文章页既有版式（纸面区块 + 主栏/侧栏）占位，正文与作者区随后各就各位。
      版式复用 tailwind.css 里的 .article-paper / .article-layout，不另起一套骨架布局。
    -->
    <p class="sr-only">文章正在加载，正文稍后出现。</p>

    <header class="article-paper article-skeleton__hero" aria-hidden="true">
      <LoadingSkeleton :rows="3" />
    </header>

    <div class="article-layout" :class="{ 'article-layout--with-outline': props.withOutline }" aria-hidden="true">
      <aside
        v-if="props.withOutline"
        class="article-aside article-skeleton__outline"
        data-role="article-outline-column-skeleton"
      >
        <section class="article-paper article-paper--state">
          <LoadingSkeleton :rows="8" variant="text" />
        </section>
      </aside>
      <div class="article-main">
        <section class="article-paper article-paper--body">
          <LoadingSkeleton :rows="9" />
        </section>
      </div>
      <aside class="article-aside">
        <!--
          右栏列序与真实版式同构（票 236 / 决定清单第 2 节）：作者简介 → 互动条 → 目录。
          切页时骨架先占住三段，数据到达后原地替换，不跳版。
        -->
        <section class="article-paper article-paper--state" data-role="article-aside-author-skeleton">
          <LoadingSkeleton :rows="3" variant="card" />
        </section>
        <section
          class="article-paper article-actions-card article-skeleton__aside-extra"
          data-role="article-aside-actions-skeleton"
        >
          <LoadingSkeleton :rows="1" variant="card" />
        </section>
        <section
          class="article-paper article-paper--state article-skeleton__aside-extra"
          data-role="article-aside-toc-skeleton"
        >
          <LoadingSkeleton :rows="5" variant="text" />
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.article-skeleton {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

/* 首屏 hero 的占位高度与真实 hero（标题 + 摘要 + 元信息）大致对齐，避免切页后二次跳版。 */
.article-skeleton__hero {
  min-height: 8.5rem;
  padding: 1.5rem;
}

/* <1024px 左栏收进「目录」抽屉：骨架也不占正文位置，避免窄屏先出一块空白。 */
.article-skeleton__outline {
  display: none;
}

/* 右栏互动条与目录在 <1024px 都不在右栏（互动条换成底部固定条、目录收进抽屉）：
   骨架也不留这两段，否则加载完成后会凭空少两块。 */
.article-skeleton__aside-extra {
  display: none;
}

@media (min-width: 1024px) {
  .article-skeleton__outline,
  .article-skeleton__aside-extra {
    display: flex;
  }
}
</style>
