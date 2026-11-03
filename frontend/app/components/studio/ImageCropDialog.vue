<script setup lang="ts">
import { NModal } from "naive-ui";
import { watch } from "vue";

import ImageCropView from "./ImageCropView.vue";
import type { ShapeFramingState, ShapeImageTarget } from "~/utils/imageShaping";

/**
 * 裁剪窗的弹层外壳（票 215）：只负责"开/关"与事件转发。
 *
 * 交互与几何在 `ImageCropView`（不依赖 naive-ui，能被组件测试单独驱动），
 * 浮层材料沿用页面既有的 NModal + preset="card"。
 *
 * 语义只有两个出口：**确认**上抛取景状态（调用方据此烤新文件再上传）；
 * 其它任何关法（取消按钮、右上角关闭、Esc、遮罩）都算取消——抛 cancel、
 * 不上传、不产生记录。点遮罩关闭被关掉：那是最容易误触的一条。
 *
 * 尺寸写明在弹层上（票 242）：naive-ui 的 `NCard` 自带 `width: 100%`、`.n-modal`
 * 本身没有宽度，不给宽度就会被最长中文提示行撑到接近窗口宽；取景框又按宽度派生
 * 高度 → 观感是"铺满窗口"。宽度封顶 560px，窄屏 92vw 恰好留出左右各 4vw 边距。
 */
const props = defineProps<{
  show: boolean;
  src?: string;
  imageWidth: number;
  imageHeight: number;
  target: ShapeImageTarget;
  ratioText: string;
  title: string;
}>();

const emit = defineEmits<{
  (event: "update:show", value: boolean): void;
  (event: "confirm", framing: ShapeFramingState): void;
  (event: "cancel"): void;
}>();

/** 同一次开关只结算一次：Esc 可能同时被裁剪窗与弹层接住（两条路径都通向取消）。 */
let settled = false;
watch(
  () => props.show,
  (visible) => {
    if (visible) settled = false;
  },
);

/** 弹层尺寸（票 242）：桌面封顶 560px，窄屏 92vw（左右各 4vw 边距），高度不出视口。 */
const modalStyle = {
  width: "min(92vw, 560px)",
  maxHeight: "calc(100dvh - 48px)",
};

function onConfirm(framing: ShapeFramingState) {
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

/** 弹层自己发起的关闭（右上角 X / Esc）也走取消：不存在"静默关掉"这种状态。 */
function onModalUpdateShow(value: boolean) {
  if (value) return;
  onCancel();
}
</script>

<template>
  <NModal
    :show="props.show"
    preset="card"
    :title="props.title"
    :bordered="false"
    :mask-closable="false"
    :style="modalStyle"
    @update:show="onModalUpdateShow">
    <ImageCropView
      :src="props.src"
      :image-width="props.imageWidth"
      :image-height="props.imageHeight"
      :target="props.target"
      :ratio-text="props.ratioText"
      @confirm="onConfirm"
      @cancel="onCancel" />
  </NModal>
</template>
