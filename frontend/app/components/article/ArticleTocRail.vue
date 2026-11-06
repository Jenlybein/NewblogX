<script setup lang="ts">
import { computed } from "vue";
import { Motion } from "motion-v";

import { SPRING_TRAVEL, SPRING_TURN, resolveSpringMotion } from "~/utils/motion";
import {
  TOC_RAIL_NODE_RADIUS_PX,
  buildTocRailPath,
  tocRailAngleAtLength,
  tocRailLengthAtY,
  tocRailReadFraction,
  type TocRailNode,
} from "~/utils/tocRail";

/**
 * 标题大纲的视觉层：一条只向下走的虚线轨道 + 每标题一个节点 + 已读段实线填充 +
 * 一枚停在当前长度、机头贴切线的小飞机（票 296）。
 *
 * 它**只负责画**：几何全部来自 `utils/tocRail`，位置来自父级给的 `activeId`
 * （父级还在上面保留 naive NAnchor 做滚动绑定与高亮）。这里既不监听滚动，也不
 * 决定"谁是当前标题"——这样视觉层换实现时行为层不受牵连。
 *
 * 为什么不用 mask 挖洞：mask id 要么写死（同页多处会互相覆盖），要么由 `useId`
 * 派生（SSR 与客户端不一致会让水合后的线突然消失）。改成"虚线打底 + 同一条路径上
 * 的实线填充"，观感是线从飞机下穿过，代价只是多一条 path。
 *
 * 弹簧只从 `utils/motion.ts` 的预设取，并统一经 `resolveSpringMotion` 解析：
 * 「减少动态」时它返回时长 0 的落位，飞机与填充直接跳到终点（不是放慢的弹簧）。
 */
const props = defineProps<{
  /** 轨道节点（已按层级缩进、已带 y 坐标）。 */
  nodes: TocRailNode[];
  /** 当前停在哪个标题上（父级已把"钉住"揉进来）。 */
  activeId: string;
  /** 是否减少动态。 */
  prefersReducedMotion: boolean;
}>();

const path = computed(() => buildTocRailPath(props.nodes));

const activeNode = computed(
  () => path.value.nodes.find((node) => node.id === props.activeId) ?? path.value.nodes[0] ?? null,
);

/** 到当前标题为止已经走过的长度（已读段）。 */
const readLength = computed(() =>
  activeNode.value ? tocRailLengthAtY(path.value, activeNode.value.y) : 0,
);

const readFraction = computed(() =>
  activeNode.value ? tocRailReadFraction(path.value, activeNode.value.y) : 0,
);

const planeAngle = computed(() => tocRailAngleAtLength(path.value, readLength.value));

/** 一位小数够用：路径是折线，角度由手算的段方向给出。 */
function format(value: number): string {
  return String(Math.round(value * 100) / 100);
}

/** 位置与已读比例都写进属性：没有 JS 时（SSR 首帧）也停在正确的地方。 */
const planeTransform = computed(() => {
  const node = activeNode.value;
  if (!node) {
    return undefined;
  }

  return `translate(${format(node.x)} ${format(node.y)}) rotate(${format(planeAngle.value)})`;
});

const fillDashoffset = computed(() => Math.round((1 - readFraction.value) * 100) / 100);

const travelTransition = computed(() => resolveSpringMotion(SPRING_TRAVEL, props.prefersReducedMotion));
const turnTransition = computed(() => resolveSpringMotion(SPRING_TURN, props.prefersReducedMotion));

/** 位移用 travel 弹簧、转向用 turn 弹簧：两套参数各管一件事。 */
const planeTransition = computed(() => ({
  x: travelTransition.value,
  y: travelTransition.value,
  rotate: turnTransition.value,
}));

/** 对外可读的动效档位：`jump` = 减少动态下直接落位。 */
const motionKind = computed(() => (props.prefersReducedMotion ? "jump" : "spring"));

function isReadNode(node: TocRailNode): boolean {
  const current = activeNode.value;
  if (!current) {
    return false;
  }
  return node.y <= current.y;
}
</script>

<template>
  <svg
    v-if="path.nodes.length"
    class="article-toc-rail"
    :width="path.width"
    :height="path.height"
    :viewBox="`0 0 ${path.width} ${path.height}`"
    aria-hidden="true"
    data-toc-rail
    :data-rail-motion="motionKind"
  >
    <!-- 轨道本体：一直虚线（2 5），已读与未读共用这条描边节奏。 -->
    <path class="article-toc-rail__track" :d="path.d" stroke-dasharray="2 5" />

    <!-- 已读段：同一条路径上的实线，用 dashoffset 截到当前长度。 -->
    <Motion
      as="path"
      class="article-toc-rail__fill"
      :d="path.d"
      pathLength="1"
      stroke-dasharray="1 1"
      :stroke-dashoffset="fillDashoffset"
      :initial="false"
      :animate="{ strokeDashoffset: fillDashoffset }"
      :transition="travelTransition"
    />

    <circle
      v-for="node in path.nodes"
      :key="node.id"
      class="article-toc-rail__node"
      :class="{ 'is-read': isReadNode(node), 'is-current': node.id === activeNode?.id }"
      :data-node-id="node.id"
      :cx="format(node.x)"
      :cy="format(node.y)"
      :r="TOC_RAIL_NODE_RADIUS_PX"
    />

    <Motion
      v-if="activeNode"
      as="g"
      class="article-toc-rail__plane"
      :data-plane-for="activeNode.id"
      :transform="planeTransform"
      :initial="false"
      :animate="{ x: activeNode.x, y: activeNode.y, rotate: planeAngle }"
      :transition="planeTransition"
    >
      <!-- 机身朝 +x 绘制，旋转后机头始终贴轨道切线。 -->
      <path class="article-toc-rail__plane-shape" d="M -9 -5 L 7 0 L -9 5 L -4 0 Z" />
    </Motion>
  </svg>
</template>

<style scoped>
.article-toc-rail {
  display: block;
  overflow: visible;
}

.article-toc-rail__track {
  fill: none;
  stroke: var(--color-line-strong);
  stroke-width: 1.25;
  stroke-linecap: round;
}

.article-toc-rail__fill {
  fill: none;
  stroke: var(--color-ink);
  stroke-width: 1.5;
  stroke-linecap: round;
}

.article-toc-rail__node {
  fill: var(--color-surface);
  stroke: var(--color-line-strong);
  stroke-width: 1.2;
}

.article-toc-rail__node.is-read {
  stroke: var(--color-ink-muted);
}

.article-toc-rail__node.is-current {
  fill: var(--color-ink);
  stroke: var(--color-ink);
}

.article-toc-rail__plane-shape {
  fill: var(--color-ink);
  stroke: var(--color-ink);
  stroke-width: 0.5;
  stroke-linejoin: round;
}
</style>
