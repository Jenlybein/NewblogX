<script setup lang="ts">
import { ref } from "vue";
import { NInput } from "naive-ui";

const props = defineProps<{
  loading?: boolean;
  title?: string;
  placeholder?: string;
  submitLabel?: string;
  showTags?: boolean;
  canCancel?: boolean;
}>();

const emit = defineEmits<{
  submit: [content: string];
  cancel: [];
}>();

const uiStore = useUiStore();
const authStore = useAuthStore();
const content = ref("");

function handleSubmit() {
  if (!authStore.isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  if (!content.value.trim()) {
    return;
  }

  emit("submit", content.value.trim());
  content.value = "";
}
</script>

<template>
  <div class="comment-composer">
    <div class="comment-composer__head">
      <div class="comment-composer__title">{{ props.title || "发表评论" }}</div>
      <span class="comment-composer__status">
        {{ authStore.isLoggedIn ? "已登录，可直接评论" : "登录后可参与讨论" }}
      </span>
    </div>

    <NInput
      v-model:value="content"
      type="textarea"
      :autosize="{ minRows: 4, maxRows: 8 }"
      :placeholder="props.placeholder || '写下你的看法，帮助更多读者补齐思路。'"
      aria-label="评论输入框"
    />

    <div class="comment-composer__footer">
      <div v-if="props.showTags !== false" class="comment-composer__tags">
        <span class="comment-tag">接口设计</span>
        <span class="comment-tag">数据流</span>
        <span class="comment-tag">分页</span>
        <span class="comment-tag">鉴权</span>
      </div>
      <div v-else />

      <div class="comment-composer__actions">
        <button v-if="props.canCancel" type="button" class="article-action article-action--compact" @click="emit('cancel')">
          取消
        </button>
        <button v-else type="button" class="article-action article-action--compact" @click="content = ''">清空</button>
        <button
          type="button"
          class="article-action article-action--primary article-action--compact"
          :disabled="Boolean(props.loading)"
          @click="handleSubmit"
        >
          {{ props.loading ? "发送中…" : props.submitLabel || "发表评论" }}
        </button>
      </div>
    </div>
  </div>
</template>
