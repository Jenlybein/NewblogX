<script setup lang="ts">
import { IconX } from "@tabler/icons-vue";
import ArticleCover from "~/components/article/ArticleCover.vue";

/**
 * 短卡片：封面（16:9）+ 两行标题 + 一行元信息（+ 可选的行内「移出」）。
 *
 * 为什么要有这个组件：浏览历史（`/history`）与个人中心收藏夹（`/studio/favorites`）
 * 展示的是同一种东西——"一篇被收进某个私人列表的文章"。两页此前各写了一份卡片，
 * 形状不同（历史是竖向栅格、收藏是横向三栏），于是同一类内容在站内有两种观感，
 * 而且横向那套在窄容器里会把封面和文字挤到一起。
 *
 * 现在只有一个形状（票 327 起）：两页都用这张卡，差异只由调用方给的内容决定
 * （元信息一行是浏览时间还是收藏时间）。
 *
 * 两处刻意收进组件内部，而不是留给调用方用插槽拼：
 *   1. **行内「移出」按钮**——插槽内容属于调用方的样式作用域，两个页面各写一遍样式
 *      就等于又造了两份形状；放进组件才能真的只有一份。
 *   2. **封面 16:9 形状**——同一形状在站内只有一个来源（ArticleCover）。
 *
 * 栅格（几列）不属于卡片：它由容器宽度决定，留在各页面里。
 *
 * `interactive` 存在的理由只有一个：历史的批量管理模式要整卡不可点（点选由勾选框
 * 接管），此时卡片只渲染为普通块，不生成任何链接。
 */
withDefaults(
  defineProps<{
    /** 文章标题；同时用作封面替代文本。 */
    title: string;
    /** 封面地址，缺省或加载失败只留一块 16:9 空白（由 ArticleCover 统一处理）。 */
    cover?: string | null;
    /** 详情页链接；`interactive` 为假时不使用。 */
    to?: string;
    /** 元信息一行（如"浏览于 …"）。空字符串则不占位。 */
    meta?: string;
    /** 是否可点击跳转（批量管理模式关掉它）。 */
    interactive?: boolean;
    /** 是否显示行内「移出」按钮。 */
    removable?: boolean;
    /** 移出按钮的禁用态（请求在途时防连点）。 */
    removeDisabled?: boolean;
    /** 移出按钮的文案（无障碍用，默认「移出」）。 */
    removeLabel?: string;
  }>(),
  {
    cover: "",
    to: "",
    meta: "",
    interactive: true,
    removable: false,
    removeDisabled: false,
    removeLabel: "移出",
  },
);

const emit = defineEmits<{ remove: [] }>();
</script>

<template>
  <article class="article-mini-card">
    <!-- 批量选择的勾选框等覆盖层：由调用方绝对定位到封面左上角。 -->
    <slot name="overlay" />

    <NuxtLink
      v-if="interactive && to"
      :to="to"
      class="article-mini-card__cover"
      :aria-label="`查看《${title}》`"
    >
      <ArticleCover :src="cover" :title="title" />
    </NuxtLink>
    <div v-else class="article-mini-card__cover" aria-hidden="true">
      <ArticleCover :src="cover" :title="title" />
    </div>

    <div class="article-mini-card__title-row">
      <NuxtLink v-if="interactive && to" :to="to" class="article-mini-card__title">
        {{ title }}
      </NuxtLink>
      <span v-else class="article-mini-card__title">{{ title }}</span>

      <button
        v-if="removable"
        type="button"
        class="article-mini-card__remove"
        :aria-label="removeLabel"
        :disabled="removeDisabled"
        @click="emit('remove')"
      >
        <IconX :size="14" aria-hidden="true" />
      </button>
    </div>

    <span v-if="meta" class="article-mini-card__meta">{{ meta }}</span>

    <div v-if="$slots.actions" class="article-mini-card__actions">
      <slot name="actions" />
    </div>
  </article>
</template>

<style scoped>
/*
 * 色值全部走语义 token（surface-raised / line / line-faint / line-strong / ink /
 * ink-muted / focus），不写裸色值（设计 token 门禁）。
 */
.article-mini-card {
  position: relative;
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

.article-mini-card:hover {
  border-color: var(--color-line-strong);
  transform: translateY(-1px);
}

/* 封面锁 16:9（与列表 / 详情同一个形状），宽度随列宽伸缩，不写任何高度档。 */
.article-mini-card__cover {
  display: block;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--color-line-faint);
}

.article-mini-card__title-row {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 4px;
}

.article-mini-card__title {
  display: -webkit-box;
  overflow: hidden;
  flex: 1 1 auto;
  color: var(--color-ink);
  font-size: 0.875rem;
  font-weight: 600;
  line-height: 1.4;
  text-decoration: none;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.article-mini-card__title:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

/* 行内「移出」：只占一个图标位，不抢标题宽度（标题自带两行截断）。 */
.article-mini-card__remove {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  background: var(--color-surface-raised);
  color: var(--color-ink-muted);
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.article-mini-card__remove:hover,
.article-mini-card__remove:focus-visible {
  border-color: var(--color-line-strong);
  color: var(--color-ink);
}

.article-mini-card__remove:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.article-mini-card__remove:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.article-mini-card__meta {
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.04em;
}

.article-mini-card__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
  padding-top: 2px;
}

/* 窄屏只放大触摸留白，不改封面形状。 */
@media (max-width: 768px) {
  .article-mini-card__remove {
    padding: 6px;
  }
}
</style>
