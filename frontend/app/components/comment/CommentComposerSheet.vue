<script setup lang="ts">
/**
 * 评论输入面板（决定清单 §4 第 19–22 条）。
 *
 * 从窗口底部居中滑出的一块纸面，宽 `min(92vw, 640px)`，**不铺满**窗口；
 * 面板外盖一层**透明点击捕获层**（不加暗色遮罩 —— 读者还要一边看正文一边写）。
 *
 * 关闭三条途径：点面板外、Esc、右上 ×。打开时不锁 body 滚动（面板只有 60dvh，
 * 上锁会让读正文的人跳位）。动效 320ms 上滑，`prefers-reduced-motion` 下直接出现。
 *
 * 输入本体复用 `CommentComposer`（它负责 textarea、提交与取消语义），本组件只提供
 * 面板这层壳：把手、标题栏、关闭按钮与遮罩。
 */
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { IconX } from "@tabler/icons-vue";
import CommentComposer from "~/components/comment/CommentComposer.vue";

const props = withDefaults(
  defineProps<{
    /** 是否展开（`v-model:show`）。 */
    show: boolean;
    /** 标题行：发表评论 / 回复 X。 */
    title?: string;
    placeholder?: string;
    submitLabel?: string;
    /** 提交在途：禁用提交按钮。 */
    loading?: boolean;
  }>(),
  {
    title: "发表评论",
    placeholder: "写下你的看法，帮助更多读者补齐思路。",
    submitLabel: "发表评论",
    loading: false,
  },
);

const emit = defineEmits<{
  "update:show": [boolean];
  submit: [string];
}>();

const composerRef = ref<InstanceType<typeof CommentComposer> | null>(null);

function close() {
  emit("update:show", false);
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    close();
  }
}

watch(
  () => props.show,
  async (open) => {
    if (typeof document === "undefined") {
      return;
    }
    if (!open) {
      document.removeEventListener("keydown", onKeydown);
      return;
    }
    // 面板打开时把焦点送进输入框：键盘用户不用再 Tab 找一次。
    document.addEventListener("keydown", onKeydown);
    await nextTick();
    composerRef.value?.focus();
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  if (typeof document !== "undefined") {
    document.removeEventListener("keydown", onKeydown);
  }
});
</script>

<template>
  <div v-if="props.show" class="comment-sheet" data-role="comment-sheet">
    <!-- 透明捕获层：点面板外任何位置都收起（不是暗色遮罩，正文仍然可读）。 -->
    <button
      type="button"
      class="comment-sheet__scrim"
      data-role="comment-sheet-scrim"
      aria-label="关闭评论输入框"
      @click="close" />

    <section class="comment-sheet__panel" role="dialog" aria-modal="false" :aria-label="title">
      <div class="comment-sheet__grabber" aria-hidden="true" />
      <button
        type="button"
        class="comment-sheet__close"
        data-role="comment-sheet-close"
        aria-label="关闭"
        @click="close">
        <IconX :size="18" aria-hidden="true" />
      </button>

      <CommentComposer
        ref="composerRef"
        :loading="props.loading"
        :title="props.title"
        :placeholder="props.placeholder"
        :submit-label="props.submitLabel"
        @submit="(content: string) => emit('submit', content)" />
    </section>
  </div>
</template>
