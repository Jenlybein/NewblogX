<script setup lang="ts">
/**
 * 审核列表的正文单元格（票 307）。
 *
 * 短稿直接给全文；长稿先给预览，审核员点「展开全文」再看整篇——列表因此
 * 既能一眼扫过，也不至于把长文挤成一小块。展开/收起只切展示长度，不请求
 * 接口（正文已经随列表返回）。
 */
import { NButton } from 'naive-ui'
import { computed, ref } from 'vue'
import {
  isReviewBodyLong,
  reviewBodyPlainText,
  reviewBodyPreview,
} from '~/utils/reviewBody'

const props = withDefaults(defineProps<{ content?: string | null }>(), { content: '' })

const expanded = ref(false)
const plain = computed(() => reviewBodyPlainText(props.content))
const isLong = computed(() => isReviewBodyLong(props.content))
const shown = computed(() =>
  expanded.value || !isLong.value ? plain.value : reviewBodyPreview(props.content),
)
</script>

<template>
  <div class="review-body">
    <p v-if="!plain" class="review-body__empty muted">无正文</p>
    <template v-else>
      <p data-role="review-body-text" class="review-body__text">{{ shown }}</p>
      <NButton v-if="isLong" text size="tiny" type="primary" @click="expanded = !expanded">
        <span data-role="review-body-toggle">{{ expanded ? '收起' : '展开全文' }}</span>
      </NButton>
    </template>
  </div>
</template>

<style scoped>
.review-body__empty,
.review-body__text {
  margin: 0;
}
.review-body__text {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
