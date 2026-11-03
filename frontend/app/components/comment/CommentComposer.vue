<script setup lang="ts">
import { ref } from "vue";
import { NInput } from "naive-ui";
import type { InputInst } from "naive-ui";
import { useReaderLoginGate } from "~/composables/useReaderLoginGate";

const props = defineProps<{
  loading?: boolean;
  title?: string;
  placeholder?: string;
  submitLabel?: string;
  showTags?: boolean;
  canCancel?: boolean;
  /**
   * 登录态由页面传入（评论框自己不查 store）：已登录时不显示任何解释性文案，
   * 未登录只保留一句「登录后可参与讨论」（票 238 范围 6）。
   */
  loggedIn?: boolean;
}>();

const emit = defineEmits<{
  submit: [content: string];
  cancel: [];
}>();

const requireLogin = useReaderLoginGate();
const content = ref("");
const composerRoot = ref<HTMLElement | null>(null);
const inputRef = ref<InputInst | null>(null);

async function handleSubmit() {
  // 登录前置（票 238 范围 7）：未登录时只弹登录框，绝不发提交请求 ——
  // 否则请求层收到鉴权失败会把 `isLoggedIn` 打成失效。
  if (!(await requireLogin())) {
    return;
  }

  if (!content.value.trim()) {
    return;
  }

  emit("submit", content.value.trim());
  content.value = "";
}

/**
 * 点楼层的「回复」后由页面调用：先把评论框滚进视口，再聚焦 textarea。
 *
 * 只动一次 scrollIntoView（block: center），不用 smooth —— 减少动态的用户不该被
 * 强制播放一段滚动动画；`focus()` 让用户可以直接打字，不用再滚回来找输入框。
 */
function focusInput() {
  composerRoot.value?.scrollIntoView({ block: "center" });
  inputRef.value?.focus();
}

/** 只聚焦、不滚动（票 236：调用方自己负责滚动位置）。 */
function focus() {
  inputRef.value?.focus();
}

defineExpose({ focusInput, focus });
</script>

<template>
  <div ref="composerRoot" class="comment-composer">
    <div class="comment-composer__head">
      <div class="comment-composer__title">{{ props.title || "发表评论" }}</div>
      <span v-if="!props.loggedIn" class="comment-composer__status">登录后可参与讨论</span>
    </div>

    <NInput
      ref="inputRef"
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
