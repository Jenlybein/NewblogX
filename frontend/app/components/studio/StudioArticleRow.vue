<script setup lang="ts">
/**
 * 我的文章 · 压缩卡（票 351，按规格重排）。
 *
 * 规格原话（US1）："cover, title, two-line abstract, status/actions under the cover,
 * tags and four counts on one baseline"。所以布局是：
 *
 *   ┌──────────┬───────────────────────────────┐
 *   │          │ 标题                          │
 *   │  封面    │ 摘要（两行）                  │
 *   │ 160×90   │                               │
 *   ├──────────┤                               │
 *   │ 状态     │                               │
 *   │ 操作     │                               │
 *   └──────────┴───────────────────────────────┘
 *   ── 同一行：标签（左） ······ 四个计数（最右）──
 *
 * 状态与操作**在封面正下方**（与封面同宽的一列），不是卡片底部；
 * 标签与计数共用**同一条基线**，计数贴在该行最右边。
 */
import { computed } from 'vue'
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from '@tabler/icons-vue'
import { formatCount } from '~/utils/format'
import type { SearchArticleItem } from '~/types/api'

const props = defineProps<{
  article: SearchArticleItem
  statusLabel: string
  statusType: 'default' | 'success' | 'warning' | 'error'
  visibilityLabel?: string
}>()

const emit = defineEmits<{
  delete: [id: string]
  edit: [id: string]
}>()

/** 摘要最多两行；没有摘要时用一句话占位，避免卡片高度塌陷。 */
const abstract = computed(() => props.article.abstract?.trim() || '这篇文章还没有摘要')

/**
 * 标签是 `{id, title}` 对象（搜索投影返回的形状），不是字符串。
 * 只取前三个：标签与计数要挤在同一行，多的折成 +N。
 */
const allTags = computed(() =>
  (props.article.tags ?? [])
    .map(tag => ({ id: String(tag.id ?? ''), title: String(tag.title ?? '').trim() }))
    .filter(tag => tag.title))
const tags = computed(() => allTags.value.slice(0, 3))
const hiddenTagCount = computed(() => Math.max(allTags.value.length - 3, 0))
</script>

<template>
  <article class="studio-article-row" :data-role="`article-row-${article.id}`">
    <!-- 左列：封面 + 紧跟其下的状态与操作（规格 US1「status/actions under the cover」）。 -->
    <div class="studio-article-row__aside">
      <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__cover" :aria-label="`打开文章：${article.title}`">
        <img v-if="article.cover" :src="article.cover" :alt="article.title" loading="lazy" />
        <span v-else class="studio-article-row__cover-empty" aria-hidden="true" />
      </NuxtLink>

      <div class="studio-article-row__status-line">
        <span class="studio-article-row__status" :class="`studio-article-row__status--${statusType}`">
          {{ statusLabel }}
        </span>
        <span v-if="visibilityLabel" class="studio-article-row__status studio-article-row__status--warning">
          {{ visibilityLabel }}
        </span>
      </div>

      <div class="studio-article-row__actions">
        <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__action" data-role="article-view">
          查看详情
        </NuxtLink>
        <button type="button" class="studio-article-row__action" data-role="article-edit" @click="emit('edit', article.id)">
          编辑
        </button>
        <button
          type="button"
          class="studio-article-row__action studio-article-row__action--danger"
          data-role="article-delete"
          @click="emit('delete', article.id)">
          删除
        </button>
      </div>
    </div>

    <div class="studio-article-row__body">
      <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__title">
        {{ article.title }}
      </NuxtLink>
      <p class="studio-article-row__abstract">{{ abstract }}</p>

      <!-- 同一条基线：标签在左，四个计数贴最右（规格 US1）。 -->
      <div class="studio-article-row__baseline">
        <div class="studio-article-row__tags">
          <span v-for="tag in tags" :key="tag.id" class="studio-article-row__tag">{{ tag.title }}</span>
          <span v-if="hiddenTagCount > 0" class="studio-article-row__tag-more">+{{ hiddenTagCount }}</span>
        </div>

        <div class="studio-article-row__counts">
          <span title="点赞"><IconThumbUp :size="13" aria-hidden="true" /> {{ formatCount(article.digg_count) }}</span>
          <span title="收藏"><IconHeart :size="13" aria-hidden="true" /> {{ formatCount(article.favor_count) }}</span>
          <span title="评论"><IconMessageCircle2 :size="13" aria-hidden="true" /> {{ formatCount(article.comment_count) }}</span>
          <span title="阅读"><IconEye :size="13" aria-hidden="true" /> {{ formatCount(article.view_count) }}</span>
        </div>
      </div>
    </div>
  </article>
</template>
