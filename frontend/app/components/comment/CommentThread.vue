<script setup lang="ts">
import { ref } from "vue";
import { NButton, NTag } from "naive-ui";
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
    <div v-for="comment in comments" :key="comment.id" class="surface-section p-5 md:p-6">
      <div class="comment-item">
        <AppAvatar :size="42" :src="comment.user_avatar" :name="comment.user_nickname" fallback="评" />
        <div class="min-w-0 flex-1">
          <div class="mb-2 flex flex-wrap items-center gap-3">
            <span class="text-base font-semibold">{{ comment.user_nickname }}</span>
            <span class="text-sm muted">{{ formatDateTimeLabel(comment.created_at) }}</span>
            <NTag v-if="comment.status === 1" size="small" round type="warning">审核中</NTag>
          </div>
          <div class="text-[15px] leading-7">
            {{ comment.content }}
          </div>
          <div class="mt-4 flex items-center gap-5 text-sm muted">
            <button type="button" @click="emit('digg', comment.id, comment.is_digg)">
              {{ comment.is_digg ? "取消点赞" : "点赞" }} {{ comment.digg_count }}
            </button>
            <button type="button" @click="emit('reply', comment.id)">回复</button>
            <button v-if="comment.reply_count > 0" type="button" @click="toggleReplies(comment.id)">
              {{ expanded[comment.id] ? "收起回复" : `展开 ${comment.reply_count} 条回复` }}
            </button>
          </div>

          <div v-if="expanded[comment.id] && comment.replies?.length" class="comment-subtree mt-5">
            <div v-for="reply in comment.replies" :key="`${comment.id}-${reply.user_id}-${reply.created_at}`" class="comment-item py-3">
              <AppAvatar :size="34" :src="reply.user_avatar" :name="reply.user_nickname" fallback="评" />
              <div class="min-w-0 flex-1">
                <div class="mb-1 flex flex-wrap items-center gap-2">
                  <span class="font-medium">{{ reply.user_nickname }}</span>
                  <span class="text-sm muted">{{ formatDateTimeLabel(reply.created_at) }}</span>
                  <span v-if="reply.reply_user_nickname" class="text-sm muted">回复 {{ reply.reply_user_nickname }}</span>
                </div>
                <div class="text-sm leading-7">{{ reply.content }}</div>
                <div class="mt-2 flex items-center gap-5 text-sm muted">
                  <button type="button">点赞 {{ reply.digg_count }}</button>
                  <button type="button" @click="emit('reply', comment.id)">回复</button>
                </div>
              </div>
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm muted">
              <span>回复第 {{ props.replyPages?.[comment.id] || 1 }} 页，已加载 {{ props.replyLoadedPages?.[comment.id] || 1 }} 页</span>
              <div class="flex items-center gap-3">
                <NButton
                  quaternary
                  size="small"
                  :disabled="!props.replyHasPrevious?.[comment.id]"
                  @click="emit('previousReplies', comment.id)"
                >
                  上一页
                </NButton>
                <NButton
                  quaternary
                  size="small"
                  :disabled="!props.replyHasNext?.[comment.id]"
                  :loading="props.loadingReplies?.[comment.id]"
                  @click="emit('nextReplies', comment.id)"
                >
                  下一页
                </NButton>
              </div>
            </div>
          </div>

          <div v-else-if="expanded[comment.id]" class="mt-4">
            <NButton text type="primary" :loading="loadingReplies?.[comment.id]" @click="emit('loadReplies', comment.id)">
              加载回复
            </NButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
