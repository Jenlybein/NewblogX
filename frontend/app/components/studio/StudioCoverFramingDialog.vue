<script setup lang="ts">
import { NModal } from "naive-ui";
import { watch } from "vue";

import StudioCoverFramingView from "./StudioCoverFramingView.vue";
import type { Framing } from "~/utils/coverFraming";

/**
 * 取景窗的弹层外壳：只负责"开/关"与事件转发。
 *
 * 分层与后台一致：几何与交互在 `StudioCoverFramingView`（不依赖 naive-ui，
 * 能被组件测试单独驱动），浮层材料沿用写作页既有的 NModal + preset="card"。
 *
 * 语义只有两个出口：**确认**上抛 `{focus_x, focus_y, zoom}`；其它任何关法
 * （取消按钮、右上角关闭、Esc）都算取消——抛 cancel、不抛取景参数。
 * 调用方据此中止本次上传，什么也不产生。
 */
const props = withDefaults(
  defineProps<{
    show: boolean;
    src: string;
    imageWidth: number;
    imageHeight: number;
    framing?: Framing | null;
  }>(),
  { framing: null },
);

const emit = defineEmits<{
  (event: "update:show", value: boolean): void;
  (event: "confirm", framing: Framing): void;
  (event: "cancel"): void;
}>();

/**
 * 同一次开关只结算一次：Esc 可能同时被取景窗与弹层接住（两条路径都通向取消），
 * 重复抛 cancel 会让调用方把"中止上传"执行两遍。重新打开时再解锁。
 */
let settled = false;
watch(
  () => props.show,
  (visible) => {
    if (visible) settled = false;
  },
);

function onConfirm(framing: Framing) {
  if (settled) return;
  settled = true;
  emit("confirm", framing);
  emit("update:show", false);
}

function onCancel() {
  if (settled) return;
  settled = true;
  emit("cancel");
  emit("update:show", false);
}

/**
 * 弹层自己发起的关闭（右上角 X / Esc）也走取消：取景窗不存在"静默关掉"
 * 这种状态——用户看到的最后一次操作必须是"确认"或"取消"之一。
 */
function onModalUpdateShow(value: boolean) {
  if (value) return;
  onCancel();
}
</script>

<template>
  <NModal
    :show="props.show"
    preset="card"
    title="取景：文章封面（固定 16:9）"
    :bordered="false"
    :mask-closable="false"
    @update:show="onModalUpdateShow">
    <StudioCoverFramingView
      :src="props.src"
      :image-width="props.imageWidth"
      :image-height="props.imageHeight"
      :framing="props.framing"
      @confirm="onConfirm"
      @cancel="onCancel" />
  </NModal>
</template>
