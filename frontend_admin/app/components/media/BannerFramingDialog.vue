<script setup lang="ts">
import { NModal } from 'naive-ui'
import { watch } from 'vue'

import BannerFramingView from './BannerFramingView.vue'
import type { Framing } from '~/utils/framing'

/**
 * 取景窗的弹层外壳（#45）：只负责"开/关"与事件转发。
 *
 * 分层的原因：几何与交互在 `BannerFramingView`（不依赖 naive-ui，能被组件测试
 * 单独驱动），浮层材料沿用后台既有的 NModal + preset="card"（见其他页面的
 * 新建用户 / 驳回文章弹窗），不另造一套浮层语言。
 *
 * 语义只有两个出口：**确认**上抛 `{focus_x, focus_y, zoom}`；**其它任何关法**
 * （取消按钮、右上角关闭、Esc）都算取消——抛 cancel、不抛取景参数。调用方据此
 * 中止本次上传，什么也不产生。点遮罩关闭被关掉：那是最容易误触的一条。
 */
const props = withDefaults(
  defineProps<{
    show: boolean
    src: string
    imageWidth: number
    imageHeight: number
    framing?: Framing | null
  }>(),
  { framing: null },
)

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'confirm', framing: Framing): void
  (event: 'cancel'): void
}>()

/**
 * 同一次开关只结算一次：Esc 可能同时被取景窗与弹层接住（两条路径都通向取消），
 * 重复抛 cancel 会让调用方把"中止上传"执行两遍。重新打开时再解锁。
 */
let settled = false
watch(
  () => props.show,
  (visible) => {
    if (visible) settled = false
  },
)

function onConfirm(framing: Framing) {
  if (settled) return
  settled = true
  emit('confirm', framing)
  emit('update:show', false)
}

/** 取消：不上抛任何取景参数。 */
function onCancel() {
  if (settled) return
  settled = true
  emit('cancel')
  emit('update:show', false)
}

/**
 * 弹层自己发起的关闭（右上角 X / Esc）也走取消：取景窗不存在"静默关掉"这种
 * 状态——运营看到的最后一次操作必须是"确认"或"取消"之一。
 */
function onModalUpdateShow(value: boolean) {
  if (value) return
  onCancel()
}
</script>

<template>
  <NModal
    :show="props.show"
    preset="card"
    class="max-w-[720px]"
    title="取景：轮播封面（固定 16:9）"
    :bordered="false"
    :mask-closable="false"
    @update:show="onModalUpdateShow">
    <BannerFramingView
      :src="props.src"
      :image-width="props.imageWidth"
      :image-height="props.imageHeight"
      :framing="props.framing"
      @confirm="onConfirm"
      @cancel="onCancel" />
  </NModal>
</template>
