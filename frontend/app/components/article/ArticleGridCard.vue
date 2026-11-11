<script setup lang="ts">
/**
 * 多列文章卡（票 345，供收藏夹 / 分类的大弹窗复用；2026-11-12 收紧）。
 *
 * 与 ArticleMiniCard 的区别：那个是"竖排短卡 + 一行 meta 文字"，用在浏览历史
 * 这种只需要"标题 + 时间"的地方；这里的卡片要回答"这篇值不值得点开"，
 * 所以带封面、日期与四个互动计数。
 *
 * 2026-11-12 三处修正：
 *  1. 计数图标换成与首页条目同一套 tabler 图标 —— 之前用 emoji（👍💬⭐👁），
 *     字形大小/基线随系统字体变，和读者站的图标对不齐；
 *  2. 计数顺序对齐首页：点赞 / 收藏 / 评论 / 阅读；
 *  3. 卡片整体收紧（封面、标题、meta 各降一档），弹窗放大后一屏能放 7 列。
 *
 * 日期只写 `2026/11/11`，不加"发布于/更新于"前缀 —— 全站同一口径
 * （见 utils/format 的 formatDateLabel：zh-CN 数字两位就是 `2026/11/11`）。
 */
import { computed } from 'vue'
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from '@tabler/icons-vue'
import { formatCount, formatDateLabel } from '~/utils/format'

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

const dateText = computed(() => (props.date ? formatDateLabel(props.date) : ''))
</script>

<template>
  <article class="article-grid-card">
    <!-- 封面与标题只写一份：有 to 时包一层链接，没有时就是普通容器。
         用 component :is 而不是把这段复制两遍 —— 复制的那份最容易在改样式时漏掉。 -->
    <component :is="to ? 'a' : 'div'" class="article-grid-card__link" :href="to || undefined">
      <div class="article-grid-card__cover">
        <img v-if="cover" :src="cover" :alt="title" loading="lazy" />
        <span v-else class="article-grid-card__cover-empty" aria-hidden="true" />
      </div>
      <h3 class="article-grid-card__title">{{ title }}</h3>
    </component>

    <footer class="article-grid-card__meta">
      <span v-if="dateText" class="article-grid-card__date">{{ dateText }}</span>
      <div class="article-grid-card__stats">
        <span title="点赞"><IconThumbUp :size="12" aria-hidden="true" /> {{ formatCount(diggCount) }}</span>
        <span title="收藏"><IconHeart :size="12" aria-hidden="true" /> {{ formatCount(favorCount) }}</span>
        <span title="评论"><IconMessageCircle2 :size="12" aria-hidden="true" /> {{ formatCount(commentCount) }}</span>
        <span title="阅读"><IconEye :size="12" aria-hidden="true" /> {{ formatCount(viewCount) }}</span>
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
