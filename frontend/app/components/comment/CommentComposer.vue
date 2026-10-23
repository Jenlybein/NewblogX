<script setup lang="ts">
import { ref } from "vue";
import { NButton, NInput, NTag } from "naive-ui";

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
  <div class="surface-section p-5 md:p-6">
    <div class="mb-4 flex items-center justify-between">
      <div class="text-base font-semibold">{{ props.title || "发表评论" }}</div>
      <div class="glass-badge">{{ authStore.isLoggedIn ? "已登录，可直接评论" : "登录后可参与讨论" }}</div>
    </div>

    <NInput
      v-model:value="content"
      type="textarea"
      :autosize="{ minRows: 4, maxRows: 8 }"
      :placeholder="props.placeholder || '写下你的看法，帮助更多读者补齐思路。'"
      aria-label="评论输入框"
    />

    <div class="mt-4 flex flex-wrap items-center justify-between gap-4">
      <div v-if="props.showTags !== false" class="flex flex-wrap gap-2">
        <NTag round size="small">接口设计</NTag>
        <NTag round size="small">数据流</NTag>
        <NTag round size="small">分页</NTag>
        <NTag round size="small">鉴权</NTag>
      </div>
      <div v-else />

      <div class="flex items-center gap-3">
        <NButton v-if="props.canCancel" quaternary @click="emit('cancel')">取消</NButton>
        <NButton v-else quaternary @click="content = ''">清空</NButton>
        <NButton type="primary" :loading="props.loading" @click="handleSubmit">
          {{ props.submitLabel || "发表评论" }}
        </NButton>
      </div>
    </div>
  </div>
</template>
