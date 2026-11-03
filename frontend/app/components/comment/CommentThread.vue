<script setup lang="ts">
import { ref } from "vue";
import {
  IconArrowBackUp,
  IconChevronDown,
  IconThumbUp,
  IconThumbUpFilled,
} from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";
import { formatDateTimeLabel } from "~/utils/format";

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

function toggleReplies(rootId: string) {
  expanded.value[rootId] = !expanded.value[rootId];
  if (expanded.value[rootId]) {
    emit("loadReplies", rootId);
  }
}
</script>

<template>
  <div class="comment-thread">
    <article v-for="comment in comments" :key="comment.id" class="comment">
      <div class="comment__head">
        <AppAvatar :size="42" :src="comment.user_avatar" :name="comment.user_nickname" fallback="评" />
        <div class="comment__identity">
          <div class="comment__meta">
            <span class="comment__author">{{ comment.user_nickname }}</span>
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

        <div v-if="expanded[comment.id] && comment.replies?.length" class="comment__replies">
          <div
            v-for="reply in comment.replies"
            :key="`${comment.id}-${reply.user_id}-${reply.created_at}`"
            class="comment comment--reply"
          >
            <div class="comment__head">
              <AppAvatar :size="32" :src="reply.user_avatar" :name="reply.user_nickname" fallback="评" />
              <div class="comment__identity">
                <div class="comment__meta">
                  <span class="comment__author">{{ reply.user_nickname }}</span>
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

          <div class="comment__pagination">
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
        </div>

        <div v-else-if="expanded[comment.id]" class="comment__empty-replies">
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
