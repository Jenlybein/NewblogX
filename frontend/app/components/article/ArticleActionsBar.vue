<script setup lang="ts">
import { computed, ref } from "vue";
import { useMagnetButtons } from "~/composables/useMagnetButtons";
import {
  IconArrowBarToDown,
  IconArrowBarToUp,
  IconHeart,
  IconHeartFilled,
  IconMessageCircle2,
  IconShare2,
  IconThumbUp,
  IconThumbUpFilled,
} from "@tabler/icons-vue";
import { describeArticleAction } from "~/utils/articleActions";
import { formatCount } from "~/utils/format";

/**
 * 阅读页互动条（票 236；2026-11-04 决定清单 §3 第 13–14 条改版）。
 *
 * 五个动作固定顺序：**点赞 / 收藏 / 评论 / 分享 / 底部**。
 * 两种形态只差容器样式，状态一律由页面注入：
 *   - `aside`：≥1024 的右栏那一份；
 *   - `hero`：<1024 时挂在 hero 下方的那一份（原来的"底部固定条"已删除，
 *     任何宽度下都只出现一份可见）。
 *
 * 为什么组件必须保持"哑"：点赞态的真相在 `useArticleDigg`（幂等目标态 + 写响应回填），
 * 收藏态在正文的 `is_favor`，两者都已有单一来源。这里再持有一份本地状态，就会出现
 * "右栏与窄屏不同步"。所以组件只有 props 进、事件出，点一下**不发请求**。
 *
 * 可见文字只有图标 + 数字（右栏 320px），语义全在 `aria-label` / `title` 上，
 * 文案由纯函数 `describeArticleAction` 生成。「底部」没有计数，只放图标。
 */
const props = withDefaults(
  defineProps<{
    /** 点赞数（含乐观值：来自页面的 `useArticleDigg`）。 */
    diggCount?: number;
    /** 收藏数（正文权威计数）。 */
    favorCount?: number;
    /** 评论数（正文权威计数）。 */
    commentCount?: number;
    /** 当前用户是否已点赞。 */
    isDigged?: boolean;
    /** 当前用户是否已收藏。 */
    isFavor?: boolean;
    /** 点赞写请求在途。 */
    likePending?: boolean;
    /** 点赞连点防抖窗口内。 */
    likeDebouncing?: boolean;
    /** aside = ≥1024 的右栏互动条；hero = <1024 挂在 hero 下方的那一份。 */
    variant?: "aside" | "hero";
  }>(),
  {
    diggCount: 0,
    favorCount: 0,
    commentCount: 0,
    isDigged: false,
    isFavor: false,
    likePending: false,
    likeDebouncing: false,
    variant: "aside",
  },
);

const emit = defineEmits<{
  like: [];
  favorite: [];
  share: [];
  comment: [];
  /** 跳到页面底部（评论区起点）。 */
  bottom: [];
  /** 回到页面顶部（正文开头）。 */
  top: [];
}>();

const likeDisabled = computed(() => props.likePending || props.likeDebouncing);

// 磁吸跟随：六个按钮各自吸附指针（不做整组平移），实现见 composables/useMagnetButtons。
// 触屏 / 减少动态下不挂监听；位移上限 8px，只写 transform。
const barRef = ref<HTMLElement | null>(null);
useMagnetButtons(barRef);

const copy = computed(() => ({
  digg: describeArticleAction({ kind: "digg", count: props.diggCount, active: props.isDigged, pending: props.likePending }),
  favor: describeArticleAction({ kind: "favor", count: props.favorCount, active: props.isFavor }),
  comment: describeArticleAction({ kind: "comment", count: props.commentCount }),
  share: describeArticleAction({ kind: "share", count: 0 }),
  bottom: describeArticleAction({ kind: "bottom", count: 0 }),
  top: describeArticleAction({ kind: "top", count: 0 }),
}));

// 禁用是视觉层，这里再挡一次：连点不该连发请求（也不该依赖浏览器对 disabled 的处理）。
function handleLike() {
  if (likeDisabled.value) {
    return;
  }
  emit("like");
}
</script>

<template>
  <div
    ref="barRef"
    class="article-actions-bar"
    :class="`article-actions-bar--${props.variant}`"
    data-role="article-actions-bar"
    :data-variant="props.variant"
  >
    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      :class="{ 'is-active': props.isDigged }"
      data-action="digg"
      :aria-label="copy.digg.label"
      :title="copy.digg.title"
      :aria-pressed="props.isDigged ? 'true' : 'false'"
      :aria-busy="props.likePending ? 'true' : 'false'"
      :disabled="likeDisabled"
      @click="handleLike"
    >
      <component :is="props.isDigged ? IconThumbUpFilled : IconThumbUp" :size="18" aria-hidden="true" />
      <span class="article-actions-bar__count" data-role="count">{{ formatCount(props.diggCount) }}</span>
    </button>

    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      :class="{ 'is-active': props.isFavor }"
      data-action="favor"
      :aria-label="copy.favor.label"
      :title="copy.favor.title"
      :aria-pressed="props.isFavor ? 'true' : 'false'"
      @click="emit('favorite')"
    >
      <component :is="props.isFavor ? IconHeartFilled : IconHeart" :size="18" aria-hidden="true" />
      <span class="article-actions-bar__count" data-role="count">{{ formatCount(props.favorCount) }}</span>
    </button>

    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      data-action="comment"
      :aria-label="copy.comment.label"
      :title="copy.comment.title"
      @click="emit('comment')"
    >
      <IconMessageCircle2 :size="18" aria-hidden="true" />
      <span class="article-actions-bar__count" data-role="count">{{ formatCount(props.commentCount) }}</span>
    </button>

    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      data-action="share"
      :aria-label="copy.share.label"
      :title="copy.share.title"
      @click="emit('share')"
    >
      <IconShare2 :size="18" aria-hidden="true" />
    </button>

    <!-- 「底部」没有计数：图标-only，语义在 aria-label / title。 -->
    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      data-action="bottom"
      :aria-label="copy.bottom.label"
      :title="copy.bottom.title"
      @click="emit('bottom')"
    >
      <IconArrowBarToDown :size="18" aria-hidden="true" />
    </button>

    <!-- 「到顶」与「到底」成对：一上一下，同一个图标家族的 bar 箭头。 -->
    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-magnet
      data-action="top"
      :aria-label="copy.top.label"
      :title="copy.top.title"
      @click="emit('top')"
    >
      <IconArrowBarToUp :size="18" aria-hidden="true" />
    </button>
  </div>
</template>
