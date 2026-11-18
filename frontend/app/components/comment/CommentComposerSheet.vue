<script setup lang="ts">
/**
 * 评论输入面板（决定清单 §4 第 19–22 条 + 票 431）。
 *
 * 从窗口底部居中滑出的一块纸面，宽 `min(92vw, 640px)`，**不铺满**窗口；
 * 面板外盖一层**透明点击捕获层**（不加暗色遮罩 —— 读者还要一边看正文一边写）。
 *
 * 关闭四条途径：点面板外、Esc、右上 ×、把手下滑过关。打开时不锁 body 滚动
 * （面板只有 60dvh，上锁会让读正文的人跳位）。
 *
 * 进场 320ms 上滑（`--motion-slow`），退场 120ms 滑走（`--motion-fast`）：
 * 退场由 `transitionend` 驱动卸载，而不是 `setTimeout` 猜时长 —— 定时器与真实
 * 过渡一旦不同步（掉帧、浏览器合并 transition），面板要么提前消失、要么卡住。
 *
 * 把手拖拽（spec 423 第 1 批第 2 项）：
 *   - 拖动期 `transition: none`，1:1 跟手（只写 `transform`）；
 *   - 松手时「位移 > 面板高度 30%」**或**「速度 > 0.11 px/ms」才关闭，否则
 *     `--motion-base`（200ms）弹回原位；判定在 `utils/commentSheetDrag.ts`；
 *   - 指针捕获走 `setPointerCapture`，多指只认第一根。
 *
 * 「减少动态」下没有退场动画：直接卸载（不是把 120ms 放慢到 0.01ms —— 那样会
 * 等一个永远不来的 `transitionend`）。
 *
 * 输入本体复用 `CommentComposer`（它负责 textarea、提交与取消语义），本组件只提供
 * 面板这层壳：把手、标题栏、关闭按钮与遮罩。
 */
import type { CSSProperties } from "vue";
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { IconX } from "@tabler/icons-vue";
import CommentComposer from "~/components/comment/CommentComposer.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { sheetDragOffset, shouldDismissSheetDrag } from "~/utils/commentSheetDrag";

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

const { prefersReducedMotion } = useMotionPreference();

const composerRef = ref<InstanceType<typeof CommentComposer> | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const handleRef = ref<HTMLElement | null>(null);

/**
 * 面板自己的视觉阶段。
 *
 * 为什么不直接用 `props.show` 决定 `v-if`：父组件收到 `update:show(false)` 会立刻
 * 把 `show` 翻成 false，而面板要先把 120ms 退场播完。所以「还在不在」由 `rendered`
 * 决定，`show` 只负责「想不想让它开着」。
 */
type SheetPhase = "idle" | "dragging" | "settling" | "leaving";

const rendered = ref(props.show);
const phase = ref<SheetPhase>("idle");
const dragOffset = ref(0);

/** 拖拽会话（非响应式：只给指针事件处理用，不需要触发渲染）。 */
let activePointerId: number | null = null;
let dragStartY = 0;
let dragStartAt = 0;
let dragPanelHeightPx = 0;

const panelStyle = computed<CSSProperties>(() => {
  if (phase.value === "leaving") {
    return {
      transform: "translateY(100%)",
      opacity: "0",
      // 退场走 fast 档（120ms）；进出对称靠的是"有退场"，不是"退场和进场一样长"。
      transition:
        "transform var(--motion-fast) var(--motion-ease), opacity var(--motion-fast) var(--motion-ease)",
    };
  }
  if (phase.value === "dragging") {
    return {
      transform: `translateY(${dragOffset.value}px)`,
      transition: "none",
    };
  }
  if (phase.value === "settling") {
    return {
      transform: "translateY(0)",
      transition: "transform var(--motion-base) var(--motion-ease)",
    };
  }
  return {};
});

/**
 * 状态类只做一件事：把进场 keyframes 停掉。
 *
 * 进场是 320ms 的上滑 `@keyframes`，如果读者在它跑完之前就关（点 × / Esc / 点外面），
 * 正在运行的动画会压过内联 `transform`，退场会先"再升一下"再跳到终点。所以拖拽 /
 * 弹回 / 退场三态都把 `animation` 归零（见 tailwind.css）。持续时长与缓动仍由内联
 * `transition` 决定，token 还是那一份。
 */
const panelStateClass = computed(() => ({
  "comment-sheet__panel--dragging": phase.value === "dragging",
  "comment-sheet__panel--settling": phase.value === "settling",
  "comment-sheet__panel--leaving": phase.value === "leaving",
}));

/** 开始退场。减少动态直接卸载；否则等 `transitionend`（见 onTransitionEnd）。 */
function startExit() {
  if (!rendered.value || phase.value === "leaving") return;

  if (prefersReducedMotion.value) {
    rendered.value = false;
    phase.value = "idle";
    dragOffset.value = 0;
    return;
  }

  phase.value = "leaving";
}

/** 三条关闭路径的唯一出口：先同步 emit（页面靠它收起 v-model），再播退场。 */
function requestClose() {
  // 退场途中重复点（捕获层还在、Esc 还挂着）不再发第二次 false。
  if (phase.value === "leaving") return;
  emit("update:show", false);
  startExit();
}

/** 退场或弹回结束：前者卸载，后者回到可以再次拖动的稳定态。 */
function onTransitionEnd() {
  if (phase.value === "leaving") {
    rendered.value = false;
    phase.value = "idle";
    dragOffset.value = 0;
    return;
  }
  if (phase.value === "settling") {
    phase.value = "idle";
  }
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    requestClose();
  }
}

function pointerIdOf(event: PointerEvent): number {
  return typeof event.pointerId === "number" ? event.pointerId : 0;
}

/** 指针捕获：手指移出把手（甚至移出面板）后事件仍然回到把手上。 */
function capturePointer(pointerId: number) {
  const handle = handleRef.value;
  if (!handle || typeof handle.setPointerCapture !== "function") return;
  try {
    handle.setPointerCapture(pointerId);
  } catch {
    // 合成事件 / 无指针设备的环境不支持捕获：退化为常规事件流，手势仍可用。
  }
}

function releasePointer(pointerId: number) {
  const handle = handleRef.value;
  if (!handle || typeof handle.releasePointerCapture !== "function") return;
  try {
    handle.releasePointerCapture(pointerId);
  } catch {
    // 浏览器在 pointerup 后已经自动释放，重复释放抛错不算失败。
  }
}

function onHandlePointerDown(event: PointerEvent) {
  // 多指：只认第一根手指，后续触点一律忽略（不重置起点、不抢捕获）。
  if (activePointerId !== null) return;
  if (event.button !== 0) return;

  activePointerId = pointerIdOf(event);
  phase.value = "dragging";
  dragOffset.value = 0;
  dragStartY = event.clientY;
  dragStartAt = event.timeStamp;
  dragPanelHeightPx = panelRef.value?.getBoundingClientRect().height ?? 0;
  capturePointer(activePointerId);
}

function onHandlePointerMove(event: PointerEvent) {
  if (phase.value !== "dragging" || activePointerId === null) return;
  if (pointerIdOf(event) !== activePointerId) return;

  dragOffset.value = sheetDragOffset(dragStartY, event.clientY);
}

function onHandlePointerUp(event: PointerEvent) {
  if (phase.value !== "dragging" || activePointerId === null) return;
  if (pointerIdOf(event) !== activePointerId) return;

  const dismiss = shouldDismissSheetDrag({
    offsetPx: dragOffset.value,
    elapsedMs: event.timeStamp - dragStartAt,
    panelHeightPx: dragPanelHeightPx,
  });

  releasePointer(activePointerId);
  activePointerId = null;

  if (dismiss) {
    // 从拖到的位置继续滑走，不先弹回 —— 否则会出现"回一下再滑走"的两段感。
    requestClose();
    return;
  }

  phase.value = "settling";
}

function onHandlePointerCancel(event: PointerEvent) {
  if (activePointerId === null) return;
  if (pointerIdOf(event) !== activePointerId) return;

  releasePointer(activePointerId);
  activePointerId = null;
  if (phase.value === "dragging") {
    phase.value = "settling";
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
      startExit();
      return;
    }
    rendered.value = true;
    phase.value = "idle";
    dragOffset.value = 0;
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
  <div v-if="rendered" class="comment-sheet" data-role="comment-sheet">
    <!-- 透明捕获层：点面板外任何位置都收起（不是暗色遮罩，正文仍然可读）。 -->
    <button
      type="button"
      class="comment-sheet__scrim"
      data-role="comment-sheet-scrim"
      aria-label="关闭评论输入框"
      @click="requestClose" />

    <section
      ref="panelRef"
      class="comment-sheet__panel"
      :class="panelStateClass"
      data-role="comment-sheet-panel"
      role="dialog"
      aria-modal="false"
      :aria-label="title"
      :style="panelStyle"
      @transitionend.self="onTransitionEnd">
      <!-- 把手：命中区比 4px 的视觉条大一圈，指针在这里按下即进入 1:1 拖拽。 -->
      <div
        ref="handleRef"
        class="comment-sheet__handle"
        data-role="comment-sheet-handle"
        aria-hidden="true"
        @pointerdown="onHandlePointerDown"
        @pointermove="onHandlePointerMove"
        @pointerup="onHandlePointerUp"
        @pointercancel="onHandlePointerCancel">
        <div class="comment-sheet__grabber" />
      </div>
      <button
        type="button"
        class="comment-sheet__close"
        data-role="comment-sheet-close"
        aria-label="关闭"
        @click="requestClose">
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
