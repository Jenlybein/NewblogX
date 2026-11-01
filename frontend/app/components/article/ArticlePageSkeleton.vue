<script setup lang="ts">
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
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

    <div class="article-layout" aria-hidden="true">
      <div class="article-main">
        <section class="article-paper article-paper--body">
          <LoadingSkeleton :rows="9" />
        </section>
      </div>
      <aside class="article-aside">
        <section class="article-paper article-paper--state">
          <LoadingSkeleton :rows="2" variant="card" />
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
</style>
