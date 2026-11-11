<script setup lang="ts">
/**
 * 我的文章 · 压缩卡（票 351，2026-11-12 重排）。
 *
 * 上一版把状态与操作放在封面正下方的一列里，于是「已发布」独自占掉一整行，
 * 卡片高度被这三行（封面 / 状态 / 操作）撑住，右侧正文却只有两行字 —— 用户
 * 的原话是"丑到没边、状态还单独占一行"。
 *
 * 现在是一张**单行密度**的卡：
 *
 *   ┌───────┬──────────────────────────────────────────────────────┐
 *   │       │ [已发布] 标题 ………………………………… 查看 编辑 删除        │
 *   │ 封面   │ 摘要（两行）                                          │
 *   │ 144px │ [标签] ………………………………………… 赞 藏 评 阅（贴最右）      │
 *   └───────┴──────────────────────────────────────────────────────┘
 *
 * 状态与标题、操作同处顶行；操作平时淡出，悬停 / 键盘聚焦才现形（触屏常显）。
 * 四个计数与标签共用同一条基线、贴最右。
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
    <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__cover" :aria-label="`打开文章：${article.title}`">
      <img v-if="article.cover" :src="article.cover" :alt="article.title" loading="lazy" />
      <span v-else class="studio-article-row__cover-empty" aria-hidden="true" />
    </NuxtLink>

    <div class="studio-article-row__body">
      <!-- 顶行：状态与标题、操作同一条基线（状态不再单独占一行）。 -->
      <div class="studio-article-row__head">
        <span class="studio-article-row__status" :class="`studio-article-row__status--${statusType}`">
          {{ statusLabel }}
        </span>
        <span v-if="visibilityLabel" class="studio-article-row__status studio-article-row__status--warning">
          {{ visibilityLabel }}
        </span>
        <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__title">
          {{ article.title }}
        </NuxtLink>

        <div class="studio-article-row__actions">
          <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__action" data-role="article-view">
            查看
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

      <p class="studio-article-row__abstract">{{ abstract }}</p>

      <!-- 同一条基线：标签在左，四个计数贴最右。 -->
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
