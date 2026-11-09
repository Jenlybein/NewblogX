<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NInput, NModal } from "naive-ui";

/**
 * 分类栏的新建 / 改名弹窗（票 325）。
 *
 * 组件只管"输入一个名字"，不做业务判断：
 *  - 去空白与空名校验在 `useProfileCategoryManage.submit()`（它还要发请求、提示文案）；
 *  - 标题随 `mode` 变，改名时把当前名字回填进来 —— 用户点"改名"时看到的是
 *    "改哪个名字"，而不是一个空框。
 *
 * 用 NModal 而不是就地展开：名字是一行文本，就地展开会把分类栏撑成表单。
 */
const props = withDefaults(
  defineProps<{
    show: boolean;
    mode?: "create" | "rename";
    /** 改名时的当前名字；新建时为空。 */
    initialTitle?: string;
    /** 提交中（按钮 loading，防重复提交）。 */
    pending?: boolean;
  }>(),
  {
    mode: "create",
    initialTitle: "",
    pending: false,
  },
);

const emit = defineEmits<{
  "update:show": [value: boolean];
  submit: [title: string];
}>();

const title = ref("");

// 每次打开都按 initialTitle 重置：上一次输到一半的内容不该跟着下一次出现。
watch(
  () => [props.show, props.initialTitle] as const,
  ([show, initial]) => {
    if (show) {
      title.value = initial;
    }
  },
  { immediate: true },
);

const modalTitle = computed(() => (props.mode === "rename" ? "分类改名" : "新建分类"));
const submitLabel = computed(() => (props.mode === "rename" ? "保存" : "新建"));
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="modalTitle"
    style="max-width: 420px"
    :mask-closable="false"
    @update:show="emit('update:show', $event)"
  >
    <NInput
      v-model:value="title"
      placeholder="分类名称"
      maxlength="40"
      show-count
      data-role="category-form-title"
      @keyup.enter="emit('submit', title)"
    />
    <p class="mt-3 text-xs text-ink-muted">
      分类名首尾空白会被忽略，不能为空；同名分类会被拒绝（删过的名字可以再用）。
    </p>

    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <NButton data-role="category-form-cancel" @click="emit('update:show', false)">取消</NButton>
        <NButton type="primary" :loading="pending" data-role="category-form-save" @click="emit('submit', title)">
          {{ submitLabel }}
        </NButton>
      </div>
    </template>
  </NModal>
</template>
