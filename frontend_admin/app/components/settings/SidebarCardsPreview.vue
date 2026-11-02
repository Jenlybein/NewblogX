<script setup lang="ts">
/**
 * 右栏预览：按首页右栏的真实固定宽度画出当前草稿。
 *
 * 数据流只有一条：`drafts` → `sidebarCardsPreviewItems` → `sidebarCardViews`（后台本地
 * 同口径实现）→ 模型 → 模板。没有"保存后再拉一次"的分支，所以未保存的标题、顺序、
 * 启停、增删都会立刻体现在这里。与读者站的一致由共享契约表的用例保证。
 *
 * 不做加载占位：草稿是本地状态，预览随时可画；加载中的空列表就显示一句说明。
 */
import { computed } from 'vue'
import { SIDEBAR_CARD_EYEBROWS, tagCloudFontSize } from '~/utils/sidebarCardViews'
import type { SidebarCardDraft } from '~/utils/sidebarCardsAdmin'
import {
  SIDEBAR_PREVIEW_WIDTH_PX,
  sidebarCardsPreviewItems,
  sidebarCardsPreviewViews,
} from '~/utils/sidebarCardsPreview'

const props = defineProps<{ drafts: SidebarCardDraft[] }>()

const views = computed(() =>
  sidebarCardsPreviewViews(sidebarCardsPreviewItems(props.drafts)),
)
</script>

<template>
  <div
    class="sidebar-preview"
    data-role="sidebar-preview"
    :data-preview-width="SIDEBAR_PREVIEW_WIDTH_PX"
    :style="{ width: `${SIDEBAR_PREVIEW_WIDTH_PX}px` }">
    <div v-if="views.length === 0" class="preview-empty" data-role="preview-empty">
      启用中的卡片有内容时，这里会按首页右栏的固定宽度渲染。
    </div>

    <section
      v-for="card in views"
      :key="card.key"
      class="preview-card"
      :data-preview-card="card.kind">
      <div class="preview-eyebrow">{{ SIDEBAR_CARD_EYEBROWS[card.kind] }}</div>
      <h3 class="preview-title" data-role="preview-card-title">{{ card.title }}</h3>

      <div v-if="card.kind === 'hot_articles'" class="preview-rows">
        <div v-for="article in card.articles" :key="article.id" class="preview-row">
          <span class="preview-row-main">{{ article.title }}</span>
          <span class="preview-row-meta">{{ article.diggCount }} 赞</span>
        </div>
      </div>

      <div v-else-if="card.kind === 'latest_comments'" class="preview-rows">
        <div v-for="comment in card.comments" :key="comment.id" class="preview-row preview-row--stacked">
          <span class="preview-row-main">{{ comment.content }}</span>
          <span class="preview-row-meta">{{ comment.userNickname }} · {{ comment.articleTitle }}</span>
        </div>
      </div>

      <div v-else-if="card.kind === 'tag_cloud'" class="preview-chips">
        <span
          v-for="tag in card.tags"
          :key="tag.id"
          class="preview-chip"
          :style="{ fontSize: tagCloudFontSize(tag.weight) }">
          {{ tag.title }}
        </span>
      </div>

      <div v-else-if="card.kind === 'recommended_collections'" class="preview-rows">
        <div
          v-for="collection in card.collections"
          :key="collection.id"
          class="preview-row preview-row--collection">
          <span class="preview-collection-cover" aria-hidden="true" />
          <span class="preview-collection-body">
            <span class="preview-row-main">{{ collection.title }}</span>
            <span v-if="collection.description" class="preview-row-meta">
              {{ collection.description }}
            </span>
            <span class="preview-row-faint">{{ collection.countLabel }}</span>
          </span>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/*
 * 预览尺寸只跟首页右栏一致（宽度由脚本里的常量给），这里只负责"看起来像右栏"的
 * 卡片密度与结构线。颜色全部走语义 token，不写裸色值。
 */
.sidebar-preview {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
  min-width: 0;
}

.preview-empty {
  border: 1px dashed var(--color-line);
  border-radius: var(--radius-lg);
  padding: 1.5rem;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.7;
}

.preview-card {
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  padding: 1rem 1.25rem;
}

.preview-eyebrow {
  color: var(--color-ink-muted);
  font-size: 0.6875rem;
  letter-spacing: 0.24em;
  text-transform: uppercase;
}

.preview-title {
  margin: 0.5rem 0 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.125rem;
  font-weight: 600;
  line-height: 1.3;
}

.preview-rows {
  display: flex;
  flex-direction: column;
  margin-top: 0.75rem;
}

.preview-row {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border-top: 1px solid var(--color-line-faint);
  padding: 0.5rem 0;
}

.preview-row:last-child {
  border-bottom: 1px solid var(--color-line-faint);
}

.preview-row--stacked {
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
}

.preview-row--collection {
  align-items: flex-start;
  justify-content: flex-start;
}

.preview-row-main {
  min-width: 0;
  overflow: hidden;
  color: var(--color-ink);
  font-size: 0.875rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-row-meta {
  flex-shrink: 0;
  color: var(--color-ink-muted);
  font-size: 0.75rem;
}

.preview-row--stacked .preview-row-meta {
  margin-top: 0.25rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-row-faint {
  margin-top: 0.25rem;
  color: var(--color-ink-faint);
  font-size: 0.75rem;
}

.preview-collection-cover {
  display: block;
  flex: 0 0 48px;
  width: 48px;
  height: 48px;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-sm);
  background: var(--color-bg-subtle);
}

.preview-collection-body {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.preview-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.preview-chip {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  padding: 0.25rem 0.625rem;
  color: var(--color-ink);
  line-height: 1.5;
}
</style>
