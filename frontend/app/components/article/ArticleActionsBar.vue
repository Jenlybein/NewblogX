<script setup lang="ts">
import { computed } from "vue";
import {
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
 * 阅读页互动条（票 236 / 决定清单第 2 节）。
 *
 * 四个动作（点赞 / 收藏 / 分享 / 评论）在**右栏**与**<1024px 底部固定条**里是同一个组件：
 * 两种变体只差容器样式（`.article-actions-bar--aside` / `--fixed`），状态一律由页面注入。
 *
 * 为什么组件必须保持"哑"：点赞态的真相在 `useArticleDigg`（票 235：幂等目标态 + 写响应回填），
 * 收藏态在正文的 `is_favor`，两者都已经有单一来源。这里再持有一份本地状态，就会出现
 * "右栏与底部条不同步"——那正是本票验收要挡住的缺陷。所以组件只有 props 进、事件出，
 * 点一下**不发请求**。
 *
 * 可见文字只有图标 + 数字（右栏 320px 宽、底部条还要让出安全区），语义全在
 * `aria-label` / `title` 上，文案由纯函数 `describeArticleAction` 生成。
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
    /** aside = 右栏互动条；fixed = <1024px 底部固定条。 */
    variant?: "aside" | "fixed";
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
}>();

const likeDisabled = computed(() => props.likePending || props.likeDebouncing);

const copy = computed(() => ({
  digg: describeArticleAction({ kind: "digg", count: props.diggCount, active: props.isDigged, pending: props.likePending }),
  favor: describeArticleAction({ kind: "favor", count: props.favorCount, active: props.isFavor }),
  share: describeArticleAction({ kind: "share", count: 0 }),
  comment: describeArticleAction({ kind: "comment", count: props.commentCount }),
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
    class="article-actions-bar"
    :class="`article-actions-bar--${props.variant}`"
    data-role="article-actions-bar"
    :data-variant="props.variant"
  >
    <button
      type="button"
      class="article-action article-actions-bar__button"
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
      data-action="share"
      :aria-label="copy.share.label"
      :title="copy.share.title"
      @click="emit('share')"
    >
      <IconShare2 :size="18" aria-hidden="true" />
    </button>

    <button
      type="button"
      class="article-action article-actions-bar__button"
      data-action="comment"
      :aria-label="copy.comment.label"
      :title="copy.comment.title"
      @click="emit('comment')"
    >
      <IconMessageCircle2 :size="18" aria-hidden="true" />
      <span class="article-actions-bar__count" data-role="count">{{ formatCount(props.commentCount) }}</span>
    </button>
  </div>
</template>
