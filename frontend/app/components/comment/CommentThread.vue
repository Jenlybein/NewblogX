<script setup lang="ts">
import { ref, watch } from "vue";
import type { CSSProperties } from "vue";
import {
  IconArrowBackUp,
  IconChevronDown,
  IconThumbUp,
  IconThumbUpFilled,
} from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";
import { formatDateTimeLabel } from "~/utils/format";
import { MOTION_ENTRANCE_OFFSET_PX, staggerDelayMs } from "~/utils/motion";

interface CommentNode extends CommentRootItem {
  replies?: CommentReplyItem[];
}

const props = defineProps<{
  comments: CommentNode[];
  loadingReplies?: Record<string, boolean>;
  replyPages?: Record<string, number>;
  replyHasPrevious?: Record<string, boolean>;
  replyHasNext?: Record<string, boolean>;
  replyLoadedPages?: Record<string, number>;
}>();

const emit = defineEmits<{
  reply: [commentId: string];
  digg: [commentId: string, isDigg: boolean];
  loadReplies: [rootId: string];
  nextReplies: [rootId: string];
  previousReplies: [rootId: string];
}>();

const expanded = ref<Record<string, boolean>>({});

/**
 * 子评论展开动效（票 294）。
 *
 * 展开 → 骨架 → 条目是**一次**高度过渡里的三段：容器常驻 DOM，折叠时 `grid-template-rows: 0fr`
 * 收成 0（同时 `inert`，不可聚焦、不进读屏）；点开只把轨道推到 1fr（base 档），
 * 骨架先占住这 3 行；数据到货后条目按索引错峰淡入 —— 只动 opacity / transform，
 * 所以**不会再改一次容器高度**。
 *
 * 节奏全部来自 `utils/motion.ts`：时长在 CSS 侧取 `--motion-base` / `--motion-fast`，
 * 逐条延迟由 `staggerDelayMs(索引, 减少动态)` 算成毫秒写进内联 style（CSS 里不写数字）。
 */
const { prefersReducedMotion } = useMotionPreference();

/** 骨架固定 3 行：与每页 3 条回复（`useArticleComments` 的 `replyPageSize`）一致。 */
const REPLY_SKELETON_ROWS = 3;

/** 展开后的**第一批**条目按索引错峰；翻页那批只做交叉淡入（不排队、不改高度）。 */
const staggerBatch = ref<Record<string, boolean>>({});

/** 条目是否已落位（false = 这一批刚从 opacity 0 起步，下一帧才翻到 1）。 */
const settled = ref<Record<string, boolean>>({});

function isOpen(rootId: string): boolean {
  return Boolean(expanded.value[rootId]);
}

function toggleReplies(rootId: string) {
  const next = !isOpen(rootId);
  expanded.value = { ...expanded.value, [rootId]: next };
  // 收起一律不排队：容器自己 120ms 收回；展开的第一批才带逐条延迟。
  staggerBatch.value = { ...staggerBatch.value, [rootId]: next };
  if (next) {
    emit("loadReplies", rootId);
  }
}

/** 一批回复的指纹（页号 + 条目 id）：它变了就说明「有一批新条目要入场」。 */
function replySignature(comment: CommentNode): string {
  const ids = (comment.replies ?? []).map((reply) => reply.id).join(",");
  return `${props.replyPages?.[comment.id] ?? 1}/${ids}`;
}

/** 上一次播过入场的批次指纹（每楼层一份）：只让**自己这批变了**的楼层重播。 */
const renderedBatch: Record<string, string> = {};

/** 首帧只登记指纹、不播入场：挂载时已经在的回复不该等到下一次数据变化才"落位"。 */
let hydrated = false;

watch(
  () => props.comments.map((comment) => replySignature(comment)),
  (signatures) => {
    // 新条目先以 opacity 0 挂载、下一帧再翻到 1：挂载即 1 的话浏览器根本不会跑过渡。
    const pending: Record<string, boolean> = {};
    props.comments.forEach((comment, index) => {
      const signature = signatures[index] ?? "";
      if (!comment.replies?.length) return;
      // 别的楼层展开/翻页不该让这一层跟着闪一下：指纹没变就不重播。
      if (renderedBatch[comment.id] === signature) return;
      renderedBatch[comment.id] = signature;
      if (!hydrated) return;
      pending[comment.id] = false;
    });
    hydrated = true;
    if (!Object.keys(pending).length) return;
    settled.value = { ...settled.value, ...pending };

    const settle = () => {
      const rest: Record<string, boolean> = {};
      for (const rootId of Object.keys(pending)) rest[rootId] = true;
      settled.value = { ...settled.value, ...rest };
    };
    // SSR / 无 rAF 的环境直接落位（那些环境没有过渡可播）。
    if (typeof requestAnimationFrame === "function") requestAnimationFrame(settle);
    else settle();
  },
  { immediate: true },
);

watch(
  () => props.replyPages,
  (next, previous) => {
    // 翻页：这一批只交叉淡入，所以关掉错峰（容器高度照旧不动）。
    const paged: Record<string, boolean> = {};
    for (const [rootId, page] of Object.entries(next ?? {})) {
      const before = (previous as Record<string, number> | undefined)?.[rootId];
      if (before !== undefined && before !== page) paged[rootId] = false;
    }
    if (Object.keys(paged).length) staggerBatch.value = { ...staggerBatch.value, ...paged };
  },
);

/**
 * 条目的内联动效样式：延迟只由动效预算模块算，组件里不出现毫秒字面量。
 *
 * 「减少动态」= 整段归零：延迟 0、没有起始态（挂载即落位），过渡交给 CSS 的
 * `prefers-reduced-motion` 兜底关掉。
 */
function replyItemStyle(rootId: string, index: number): CSSProperties {
  const delayMs = staggerBatch.value[rootId] ? staggerDelayMs(index, prefersReducedMotion.value) : 0;
  if (prefersReducedMotion.value) {
    return { transitionDelay: `${delayMs}ms` };
  }

  const isSettled = settled.value[rootId] !== false;
  return {
    opacity: isSettled ? "1" : "0",
    transform: isSettled ? "none" : `translateY(${MOTION_ENTRANCE_OFFSET_PX}px)`,
    transitionDelay: `${delayMs}ms`,
  };
}

/** 骨架只在「已展开 + 这一楼层正在取数 + 还没有任何条目」时出现。 */
function showSkeleton(comment: CommentNode): boolean {
  return isOpen(comment.id) && !comment.replies?.length && Boolean(props.loadingReplies?.[comment.id]);
}

/**
 * 兜底入口在**容器之外**：容器折叠即 0 高度，失败重试若放在里面就会一起消失
 * （票 294 之前它就是「展开后唯一的可见内容」，不能因为改成常驻容器而变空白）。
 */
function showFallback(comment: CommentNode): boolean {
  return isOpen(comment.id) && !comment.replies?.length && !showSkeleton(comment);
}
</script>

<template>
  <div class="comment-thread">
    <article v-for="comment in comments" :key="comment.id" class="comment">
      <div class="comment__head">
        <!-- 作者区是进个人主页的入口（票 292）：头像与昵称包在**同一个** NuxtLink 里，
             缺失 user_id 时退化成静态文本（不可点、不可聚焦、读屏不当链接）——
             与文章卡片的作者区同一范式（ArticleFeedItem.vue）。 -->
        <NuxtLink v-if="comment.user_id" :to="`/users/${comment.user_id}`" class="comment__author-link">
          <AppAvatar :size="42" :src="comment.user_avatar" :name="comment.user_nickname" fallback="评" />
          <span class="comment__author">{{ comment.user_nickname }}</span>
        </NuxtLink>
        <span v-else class="comment__author-link comment__author-link--static">
          <AppAvatar :size="42" :src="comment.user_avatar" :name="comment.user_nickname" fallback="评" />
          <span class="comment__author">{{ comment.user_nickname }}</span>
        </span>
        <div class="comment__identity">
          <div class="comment__meta">
            <span class="comment__time">{{ formatDateTimeLabel(comment.created_at) }}</span>
            <span v-if="comment.status === 1" class="comment__status">审核中</span>
          </div>
        </div>
      </div>

      <div class="comment__body">
        <p class="comment__content">{{ comment.content }}</p>

        <div class="comment__actions">
          <!-- 点赞是图标版（票 238 范围 4）：实心/空心两态 + 数字；无障碍名靠 aria-label
               （可见内容只剩数字，光靠它读屏读不出"点赞"）。 -->
          <button
            type="button"
            class="comment-action comment-action--digg"
            :class="{ 'comment-action--active': comment.is_digg }"
            :aria-pressed="Boolean(comment.is_digg)"
            :aria-label="comment.is_digg ? '取消点赞' : '点赞'"
            @click="emit('digg', comment.id, Boolean(comment.is_digg))"
          >
            <IconThumbUpFilled v-if="comment.is_digg" :size="16" aria-hidden="true" />
            <IconThumbUp v-else :size="16" aria-hidden="true" />
            <span class="comment-action__count">{{ comment.digg_count }}</span>
          </button>
          <button
            type="button"
            class="comment-action comment-action--reply"
            aria-label="回复"
            @click="emit('reply', comment.id)"
          >
            <IconArrowBackUp :size="16" aria-hidden="true" />
            <span>回复</span>
          </button>
          <!-- 「展开 N 条回复」是胶囊按钮（票 238 范围 5）：浅底 + 主题色文字 + chevron。 -->
          <button
            v-if="comment.reply_count > 0"
            type="button"
            class="comment-capsule"
            :class="{ 'comment-capsule--expanded': expanded[comment.id] }"
            :aria-expanded="Boolean(expanded[comment.id])"
            @click="toggleReplies(comment.id)"
          >
            <span>{{ expanded[comment.id] ? "收起回复" : `展开 ${comment.reply_count} 条回复` }}</span>
            <IconChevronDown :size="14" class="comment-capsule__chevron" aria-hidden="true" />
          </button>
        </div>

        <!-- 回复容器（票 294）：常驻 DOM 才有高度可过渡；折叠时靠 0fr 轨道收成 0，
             并整体 inert（不可聚焦、不被读屏遍历）。骨架 → 条目的入场只动
             opacity/transform，容器高度全程只过渡这一次。 -->
        <div
          class="comment__replies"
          :class="{ 'comment__replies--open': isOpen(comment.id) }"
          :inert="isOpen(comment.id) ? undefined : true"
          :aria-hidden="isOpen(comment.id) ? undefined : 'true'"
        >
          <div class="comment__replies-clip">
            <div class="comment__replies-body">
              <!-- 骨架先占住这 3 行（与每页 3 条一致），数据到货后立刻被真条目替换。 -->
              <LoadingSkeleton
                v-if="showSkeleton(comment)"
                :rows="REPLY_SKELETON_ROWS"
                variant="card"
                label="回复正在加载"
              />
              <template v-else>
                <div
                  v-for="(reply, index) in comment.replies"
                  :key="`${comment.id}-${reply.user_id}-${reply.created_at}`"
                  class="comment comment--reply comment__replies-item"
                  :style="replyItemStyle(comment.id, index)"
                >
                  <div class="comment__head">
                    <!-- 子评论的作者区与根评论同一口径（票 292）；「回复 XXX」是关系说明，不是身份入口，不做链接。 -->
                    <NuxtLink v-if="reply.user_id" :to="`/users/${reply.user_id}`" class="comment__author-link">
                      <AppAvatar :size="32" :src="reply.user_avatar" :name="reply.user_nickname" fallback="评" />
                      <span class="comment__author">{{ reply.user_nickname }}</span>
                    </NuxtLink>
                    <span v-else class="comment__author-link comment__author-link--static">
                      <AppAvatar :size="32" :src="reply.user_avatar" :name="reply.user_nickname" fallback="评" />
                      <span class="comment__author">{{ reply.user_nickname }}</span>
                    </span>
                    <div class="comment__identity">
                      <div class="comment__meta">
                        <span class="comment__time">{{ formatDateTimeLabel(reply.created_at) }}</span>
                        <span v-if="reply.reply_user_nickname" class="comment__time">回复 {{ reply.reply_user_nickname }}</span>
                      </div>
                    </div>
                  </div>
                  <div class="comment__body">
                    <p class="comment__content">{{ reply.content }}</p>
                    <div class="comment__actions">
                      <!-- 子评论点赞（票 238 范围 1）：后端 `POST /comments/{id}/digg` 不区分层级，
                           曾经这里只是纯文本，所以二级点赞根本点不了。
                           回复（票 238 范围 2）传的是**被回复的这条**的 id，不是根评论 id。 -->
                      <button
                        type="button"
                        class="comment-action comment-action--digg"
                        :class="{ 'comment-action--active': reply.is_digg }"
                        :aria-pressed="Boolean(reply.is_digg)"
                        :aria-label="reply.is_digg ? '取消点赞' : '点赞'"
                        @click="emit('digg', reply.id, Boolean(reply.is_digg))"
                      >
                        <IconThumbUpFilled v-if="reply.is_digg" :size="15" aria-hidden="true" />
                        <IconThumbUp v-else :size="15" aria-hidden="true" />
                        <span class="comment-action__count">{{ reply.digg_count }}</span>
                      </button>
                      <button
                        type="button"
                        class="comment-action comment-action--reply"
                        aria-label="回复"
                        @click="emit('reply', reply.id)"
                      >
                        <IconArrowBackUp :size="15" aria-hidden="true" />
                        <span>回复</span>
                      </button>
                    </div>
                  </div>
                </div>

                <!-- 翻页：只换这一页的条目（交叉淡入），容器与轨道一律不动。 -->
                <div v-if="comment.replies?.length" class="comment__pagination">
                  <span>
                    回复第 {{ props.replyPages?.[comment.id] || 1 }} 页，已加载 {{ props.replyLoadedPages?.[comment.id] || 1 }} 页
                  </span>
                  <div class="comment__pagination-actions">
                    <button
                      type="button"
                      class="comment-page-button"
                      :disabled="!props.replyHasPrevious?.[comment.id]"
                      @click="emit('previousReplies', comment.id)"
                    >
                      上一页
                    </button>
                    <button
                      type="button"
                      class="comment-page-button"
                      :disabled="!props.replyHasNext?.[comment.id] || Boolean(props.loadingReplies?.[comment.id])"
                      @click="emit('nextReplies', comment.id)"
                    >
                      下一页
                    </button>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>

        <!-- 兜底入口在容器**之外**：容器折叠即 0 高度，重试按钮若放里面会跟着一起消失。 -->
        <div v-if="showFallback(comment)" class="comment__empty-replies">
          <button
            type="button"
            class="comment-page-button"
            :disabled="Boolean(loadingReplies?.[comment.id])"
            @click="emit('loadReplies', comment.id)"
          >
            {{ loadingReplies?.[comment.id] ? "加载中…" : "加载回复" }}
          </button>
        </div>
      </div>
    </article>
  </div>
</template>
