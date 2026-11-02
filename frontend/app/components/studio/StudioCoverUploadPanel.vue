<script setup lang="ts">
import { computed, ref } from "vue";

import type { CSSProperties } from "vue";
import { COVER_FORMAT_HINT, describeCoverBadge, formatCoverSize, type CoverUploadRoute } from "~/utils/studioCover";
import type { Size } from "~/utils/coverFraming";

/**
 * 文章封面上传面板卡：与后台轮播上传同一套语言。
 *
 * 只有"显示一个值 / 上抛一个动作"，没有自己的比例判断：走哪条路、徽标文案
 * 都由 `~/utils/studioCover` 决定，父级把结果作为 props 传进来。这样
 * "面板说不合规、上传却放行"这类分叉不可能发生。
 *
 * 交互全部走原生 button（与取景窗一致）：忙碌态可以用组件测试单独驱动，
 * 不必挂整套 naive-ui 弹层；颜色与圆角仍取同一套语义 token。
 */

const props = withDefaults(
  defineProps<{
    /** 上传在途：卡片进忙碌态，选择入口锁住。 */
    busy: boolean;
    /** 已选/已传封面的预览地址。 */
    previewUrl: string;
    /** 上传阶段文案（只在该次上传进行中或刚完成时出现）。 */
    stage: string;
    /** 留在面板里的错误文案（比一闪而过的 toast 更容易看清）。 */
    error: string;
    /** 选过图之后才有原图尺寸。 */
    size: Size | null;
    /** 选过图之后才有上传路径；空串＝还没选图（回填的旧封面也没有尺寸）。 */
    route: CoverUploadRoute | "";
    /** 取景参数是否已经被确认过。 */
    framed: boolean;
    /** 预览图的取景样式（与取景窗同一出口）。 */
    previewStyle?: CSSProperties | null;
    /** 原生 change 事件原样交给父级（上传逻辑不在本组件）。 */
    onFileChange: (event: Event) => void;
    /** 移除已选封面。 */
    onClear: () => void;
    /** 打开取景窗（手动取景 / 调整取景）。 */
    onFrame: () => void;
  }>(),
  { previewStyle: null },
);

const fileInput = ref<HTMLInputElement | null>(null);

const badge = computed(() => (props.route ? describeCoverBadge({ route: props.route, framed: props.framed }) : null));
const sizeLabel = computed(() => formatCoverSize(props.size));
const canFrame = computed(() => props.route === "frame" || props.framed);
const frameLabel = computed(() => (props.framed ? "调整取景" : "手动取景"));

/** 替父级点开文件选择：input 在本组件内，让父级持 ref 再回来点它没有意义。 */
function pickCover() {
  if (props.busy) return;
  fileInput.value?.click();
}
</script>

<template>
  <div class="cover-panel">
    <div class="cover-panel__head">
      <div>
        <div class="cover-panel__title">文章封面</div>
        <p class="cover-panel__hint">
          推荐比例 16:9：与 16:9 相差 ≤1% 会立即上传；超出 1% 先取景，确认后自动上传
        </p>
      </div>
      <div class="cover-panel__actions">
        <button
          v-if="canFrame"
          type="button"
          class="cover-panel__action"
          :aria-label="frameLabel"
          :disabled="props.busy"
          @click="props.onFrame">
          {{ frameLabel }}
        </button>
        <button
          type="button"
          class="cover-panel__action"
          aria-label="选择图片"
          :disabled="props.busy"
          :aria-busy="props.busy"
          @click="pickCover">
          {{ props.previewUrl ? "重新选择" : "选择图片" }}
        </button>
        <button
          v-if="props.previewUrl"
          type="button"
          class="cover-panel__action cover-panel__remove"
          aria-label="移除封面"
          :disabled="props.busy"
          @click="props.onClear">
          移除
        </button>
      </div>
    </div>

    <input
      ref="fileInput"
      type="file"
      accept="image/*"
      class="cover-panel__input"
      @change="props.onFileChange">

    <button
      type="button"
      class="cover-panel__picker"
      :class="{ 'cover-panel__picker--busy': props.busy }"
      :aria-busy="props.busy"
      :aria-label="props.previewUrl ? '更换封面图片' : '上传封面'"
      @click="pickCover">
      <img
        v-if="props.previewUrl"
        :src="props.previewUrl"
        :style="props.previewStyle ?? undefined"
        alt="文章封面预览"
        class="cover-panel__image">
      <template v-else>
        <span class="cover-panel__plus" aria-hidden="true">+</span>
        <span class="cover-panel__cta">上传封面</span>
        <small class="cover-panel__format">{{ COVER_FORMAT_HINT }}</small>
      </template>
      <span v-if="props.busy" class="cover-panel__busy" role="status">正在上传…</span>
    </button>

    <div v-if="props.size || badge" class="cover-panel__meta">
      <span v-if="props.size" class="cover-panel__size">{{ sizeLabel }}</span>
      <span
        v-if="badge"
        class="cover-panel__badge"
        :class="`cover-panel__badge--${badge.tone}`">
        {{ badge.text }}
      </span>
    </div>

    <p v-if="props.stage" class="cover-panel__stage" role="status">{{ props.stage }}</p>
    <p v-if="props.error" class="cover-panel__error" role="alert">{{ props.error }}</p>
  </div>
</template>

<style scoped>
/* 与后台轮播上传面板同一张卡：抬升纸面 + 虚线取景入口；颜色只消费语义 token。 */
.cover-panel {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: linear-gradient(180deg, var(--color-surface-raised), var(--color-bg-subtle));
}

.cover-panel__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.cover-panel__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-ink);
}

.cover-panel__hint {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-ink-muted);
}

.cover-panel__actions {
  display: flex;
  flex-shrink: 0;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.cover-panel__action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-ink);
  font-size: 13px;
  cursor: pointer;
  transition:
    background-color var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
}

.cover-panel__action:hover {
  border-color: var(--color-line);
  background: var(--color-surface-raised);
}

.cover-panel__action:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.cover-panel__input {
  display: none;
}

.cover-panel__picker {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 168px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  overflow: hidden;
  aspect-ratio: 16 / 9;
  border: 1px dashed var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  color: var(--color-ink-muted);
  font-size: 14px;
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    background-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.cover-panel__picker:hover {
  border-color: var(--color-line-strong);
  background: var(--color-bg-subtle);
  color: var(--color-ink);
}

.cover-panel__picker--busy {
  opacity: 0.72;
  cursor: wait;
}

.cover-panel__image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-panel__plus {
  font-size: 36px;
  line-height: 1;
}

.cover-panel__cta {
  font-weight: 600;
}

.cover-panel__format {
  font-size: 12px;
}

.cover-panel__busy {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-surface-glass);
  font-size: 13px;
  color: var(--color-ink);
}

.cover-panel__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--color-ink-muted);
}

.cover-panel__badge {
  display: inline-flex;
  align-items: center;
  min-height: 22px;
  padding: 0 8px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  font-size: 12px;
  background: var(--color-surface-raised);
  color: var(--color-ink);
}

.cover-panel__badge--warning {
  border-color: var(--color-danger-line);
  background: var(--color-danger-soft);
  color: var(--color-danger-ink);
}

.cover-panel__stage {
  margin: 0;
  font-size: 12px;
  color: var(--color-ink-muted);
}

.cover-panel__error {
  margin: 0;
  font-size: 12px;
  color: var(--color-danger-ink);
}
</style>
