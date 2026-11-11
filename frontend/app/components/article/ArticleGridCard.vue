<script setup lang="ts">
/**
 * 多列文章卡（票 345，供收藏夹/分类的大弹窗复用）。
 *
 * 与 ArticleMiniCard 的区别：那个是"竖排短卡 + 一行 meta 文字"，用在浏览历史
 * 这种只需要"标题 + 时间"的地方；这里的卡片要回答"这篇值不值得点开"，
 * 所以带封面、日期与四个互动计数。
 *
 * 日期只写 `2026/11/11`，不加"发布于/更新于"前缀 —— 全站同一口径
 * （见 utils/format 的 dateOnlyLabel）。
 */
import { computed } from 'vue'
import { formatCount } from '~/utils/format'
import { dateOnlyLabel } from '~/utils/format'

const props = withDefaults(defineProps<{
  title: string
  cover?: string
  /** ISO 时间串；为空时整行日期不渲染。 */
  date?: string
  to?: string
  viewCount?: number
  diggCount?: number
  commentCount?: number
  favorCount?: number
  /** hover 时才出现的操作按钮文案（例如「移出收藏夹」）；为空则不显示。 */
  actionLabel?: string
}>(), {
  cover: '',
  date: '',
  to: '',
  viewCount: 0,
  diggCount: 0,
  commentCount: 0,
  favorCount: 0,
  actionLabel: '',
})

const emit = defineEmits<{ action: [] }>()

const dateText = computed(() => (props.date ? dateOnlyLabel(props.date) : ''))
</script>

<template>
  <article class="article-grid-card">
    <a v-if="to" class="article-grid-card__link" :href="to">
      <div class="article-grid-card__cover">
        <img v-if="cover" :src="cover" :alt="title" loading="lazy" />
        <span v-else class="article-grid-card__cover-empty" aria-hidden="true" />
      </div>
      <h3 class="article-grid-card__title">{{ title }}</h3>
    </a>
    <div v-else class="article-grid-card__link">
      <div class="article-grid-card__cover">
        <img v-if="cover" :src="cover" :alt="title" loading="lazy" />
        <span v-else class="article-grid-card__cover-empty" aria-hidden="true" />
      </div>
      <h3 class="article-grid-card__title">{{ title }}</h3>
    </div>

    <footer class="article-grid-card__meta">
      <span v-if="dateText" class="article-grid-card__date">{{ dateText }}</span>
      <div class="article-grid-card__stats">
        <span title="点赞"><span aria-hidden="true">👍</span> {{ formatCount(diggCount) }}</span>
        <span title="评论"><span aria-hidden="true">💬</span> {{ formatCount(commentCount) }}</span>
        <span title="收藏"><span aria-hidden="true">⭐</span> {{ formatCount(favorCount) }}</span>
        <span title="阅读"><span aria-hidden="true">👁</span> {{ formatCount(viewCount) }}</span>
      </div>
    </footer>

    <button
      v-if="actionLabel"
      type="button"
      class="article-grid-card__action"
      @click.stop="emit('action')">
      {{ actionLabel }}
    </button>
  </article>
</template>
