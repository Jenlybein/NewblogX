<script setup lang="ts">
import { NInput } from "naive-ui";

/**
 * 图形验证码控件（纯展示）：一张可点击刷新的图 + 一个输入框。
 *
 * 状态由 `useCaptchaProof()` 持有，这里不取数、不做校验——四个入口的表现必须一致，
 * 逻辑只能有一份。
 */
defineProps<{
  image: string;
  code: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  "update:code": [string];
  refresh: [];
}>();
</script>

<template>
  <div class="flex items-center gap-3">
    <button
      type="button"
      class="shrink-0 cursor-pointer rounded-field p-0"
      :disabled="loading"
      title="看不清？点一下换一张"
      aria-label="刷新图形验证码"
      data-role="captcha-image"
      @click="emit('refresh')"
    >
      <img v-if="image" :src="image" alt="图形验证码" class="h-[40px] w-auto rounded-field" />
      <span v-else class="flex h-[40px] w-[100px] items-center justify-center text-xs muted">
        {{ loading ? "加载中…" : "点此重试" }}
      </span>
    </button>
    <NInput
      :value="code"
      maxlength="6"
      placeholder="图形验证码"
      data-role="captcha-input"
      @update:value="value => emit('update:code', value)"
    />
  </div>
</template>
