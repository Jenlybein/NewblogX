<script setup lang="ts">
/**
 * 我的文章 · 压缩行卡（票 351）。
 *
 * 用户原话：列表项高度太高、"多余的行"太多，希望把状态与操作、四个计数
 * 都收进同一张卡里。所以这里只有**一层**：封面在左（160×90 固定），
 * 右侧一列从上到下是标题 / 摘要（两行）/ 标签 / 底部一行。
 * 底部那一行左边是状态与操作、右边是四个计数 —— 与标签同列，
 * 计数贴在该行最右边。
 *
 * 为什么不复用 ArticleFeedItem：那张卡是"读者视角"的通用条目，作者视角需要
 * 状态与编辑/删除按钮，硬塞进去会让读者页也背上作者操作。这里另起一张，
 * 只给创作页用。
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

const article = computed(() => props.article)

/** 摘要最多两行；没有摘要时用一句话占位，避免卡片高度塌陷。 */
const abstract = computed(() => props.article.abstract?.trim() || '这篇文章还没有摘要')

/**
 * 标签是 `{id, title}` 对象（搜索投影返回的形状），不是字符串。
 * 只取前三个：卡片要压缩高度，标签一行放得下就够了，多的折成 +N。
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
      <NuxtLink :to="`/article/${article.id}`" class="studio-article-row__title">
        {{ article.title }}
      </NuxtLink>
      <p class="studio-article-row__abstract">{{ abstract }}</p>

      <div v-if="tags.length" class="studio-article-row__tags">
        <span v-for="tag in tags" :key="tag.id" class="studio-article-row__tag">{{ tag.title }}</span>
        <span v-if="hiddenTagCount > 0" class="studio-article-row__tag-more">+{{ hiddenTagCount }}</span>
      </div>

      <div class="studio-article-row__foot">
        <div class="studio-article-row__left">
          <span class="studio-article-row__status" :class="`studio-article-row__status--${statusType}`">
            {{ statusLabel }}
          </span>
          <span v-if="visibilityLabel" class="studio-article-row__status studio-article-row__status--warning">
            {{ visibilityLabel }}
          </span>

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
