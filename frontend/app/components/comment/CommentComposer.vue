<script setup lang="ts">
import { ref } from "vue";
import { NInput } from "naive-ui";
import type { InputInst } from "naive-ui";
import { useReaderLoginGate } from "~/composables/useReaderLoginGate";

/**
 * 评论输入本体（2026-11-04 决定清单 §4）。
 *
 * 本轮它不再是"页面里内嵌的一块大 textarea"：内容这套壳由 `CommentComposerSheet`
 * 提供（底部滑出面板），这里只负责**输入 + 提交**。
 *
 * 因此两样东西一并退役：
 *  - 写死的装饰标签行（接口设计 / 数据流 / 分页 / 鉴权）—— 假数据会让人以为能选标签；
 *  - 「登录后可参与讨论」提示 —— 未登录时点入口就弹登录框，语义自己说明了。
 */
const props = defineProps<{
  loading?: boolean;
  title?: string;
  placeholder?: string;
  submitLabel?: string;
}>();

const emit = defineEmits<{
  submit: [content: string];
  cancel: [];
}>();

const requireLogin = useReaderLoginGate();
const content = ref("");
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

/** 只聚焦、不滚动（焦点由面板负责，页面不再滚动到这里）。 */
function focus() {
  inputRef.value?.focus();
}

defineExpose({ focus });
</script>

<template>
  <div class="comment-composer comment-composer--sheet">
    <div class="comment-composer__head">
      <div class="comment-composer__title">{{ props.title || "发表评论" }}</div>
    </div>

    <NInput
      ref="inputRef"
      v-model:value="content"
      type="textarea"
      :autosize="{ minRows: 3, maxRows: 6 }"
      :placeholder="props.placeholder || '写下你的看法，帮助更多读者补齐思路。'"
      aria-label="评论输入框" />

    <div class="comment-composer__footer">
      <div />
      <div class="comment-composer__actions">
        <button
          type="button"
          class="article-action article-action--primary article-action--compact"
          data-role="comment-submit"
          :disabled="Boolean(props.loading)"
          @click="handleSubmit"
        >
          {{ props.loading ? "发送中…" : props.submitLabel || "发表评论" }}
        </button>
      </div>
    </div>
  </div>
</template>
