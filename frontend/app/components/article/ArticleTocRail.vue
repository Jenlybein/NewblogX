<script setup lang="ts">
import { computed } from "vue";
import { Motion } from "motion-v";

import { SPRING_TRAVEL, SPRING_TURN, resolveSpringMotion } from "~/utils/motion";
import {
  TOC_RAIL_NODE_RADIUS_HOLLOW_PX,
  TOC_RAIL_NODE_RADIUS_PX,
  buildTocRailPath,
  tocRailLengthAtY,
  tocRailPlaneRotationDeg,
  tocRailPointAtLength,
  tocRailReadFraction,
  type TocRailNode,
} from "~/utils/tocRail";

/**
 * 标题大纲的视觉层：一条**会拐弯**的虚线轨道 + 每标题一个节点 + 已读段实线填充 +
 * 一枚停在当前长度、机头贴切线的小飞机（票 296，返工版）。
 *
 * 它**只负责画**：几何全部来自 `utils/tocRail`，位置来自父级给的 `activeId`
 * （父级还在上面保留 naive NAnchor 做滚动绑定与高亮）。这里既不监听滚动，也不
 * 决定"谁是当前标题"——这样视觉层换实现时行为层不受牵连。
 *
 * 三处取自参考组件（React 版 RailToc）的形态：
 *   1. 轨道是**折线**：节点 x 随层级变，相邻节点之间在 30% 处拐弯（几何在 utils 里算）；
 *   2. 虚线打底（`2 5`）+ 同一条路径上的实线按 `pathLength` 截到已读长度；
 *   3. 飞机是一枚 16px 的矢量（机头朝上画），按"切线 + 90°"转向后停当前长度处。
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

/** 飞机矢量：与参考组件同一枚（视口 24），机头在正上方。 */
const PLANE_GLYPH = "M12 2 20.5 21 12 17.5 3.5 21z";
const PLANE_GLYPH_BOX_PX = 24;
/** 画到画布上的实际尺寸（px）：轨道带宽 44，16 的机身不越界。 */
const PLANE_GLYPH_SIZE_PX = 16;

/** 把 24 视口的机身缩放成 16、并把机身中心挪到原点（旋转/平移都绕中心发生）。 */
const PLANE_GLYPH_TRANSFORM =
  `scale(${PLANE_GLYPH_SIZE_PX / PLANE_GLYPH_BOX_PX}) ` +
  `translate(${-PLANE_GLYPH_BOX_PX / 2} ${-PLANE_GLYPH_BOX_PX / 2})`;

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

/** 飞机停机点 = 已读长度的末端；没有节点就没有可落的点。 */
const planePoint = computed(() => tocRailPointAtLength(path.value, readLength.value));

const planeRotation = computed(() => tocRailPlaneRotationDeg(path.value, readLength.value));

/** 两位小数够用：几何本身是手算的折线，不需要亚像素精度。 */
function format(value: number): string {
  return String(Math.round(value * 100) / 100);
}

/** 位置与角度都写进属性：没有 JS 时（SSR 首帧）也停在正确的地方。 */
const planeTransform = computed(() => {
  const point = planePoint.value;
  if (!point) {
    return undefined;
  }

  return `translate(${format(point.x)} ${format(point.y)}) rotate(${format(planeRotation.value)})`;
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

/** 已读 = 当前标题及其上方：参考组件把已读画成实心小圆、未读画成空心。 */
function isReadNode(node: TocRailNode): boolean {
  const current = activeNode.value;
  if (!current) {
    return false;
  }
  return node.y <= current.y;
}

/** 半径随已读态变：实心 3px、空心 3.25px（空心多 0.25 才不显细）。 */
function nodeRadius(node: TocRailNode): number {
  return isReadNode(node) ? TOC_RAIL_NODE_RADIUS_PX : TOC_RAIL_NODE_RADIUS_HOLLOW_PX;
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
    <!-- 轨道本体：折线 + 始终虚线（2 5），已读与未读共用这条描边节奏。 -->
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
      :r="nodeRadius(node)"
    />

    <Motion
      v-if="planePoint"
      as="g"
      class="article-toc-rail__plane"
      :data-plane-for="activeNode?.id"
      :transform="planeTransform"
      :initial="false"
      :animate="{ x: planePoint.x, y: planePoint.y, rotate: planeRotation }"
      :transition="planeTransition"
    >
      <!-- 机身朝上画；父级旋转已含 +90°，旋转后机头贴轨道切线。 -->
      <g :transform="PLANE_GLYPH_TRANSFORM">
        <path class="article-toc-rail__plane-shape" :d="PLANE_GLYPH" />
      </g>
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
  stroke-linejoin: round;
}

.article-toc-rail__fill {
  fill: none;
  stroke: var(--color-ink);
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 未读：空心圆，填充用纸面色把底下的虚线隔断，才看得出是"空心"而不是"线穿点"。 */
.article-toc-rail__node {
  fill: var(--color-surface);
  stroke: var(--color-line-strong);
  stroke-width: 1.4;
}

.article-toc-rail__node.is-read {
  fill: var(--color-ink-muted);
  stroke: var(--color-ink-muted);
}

.article-toc-rail__node.is-current {
  fill: var(--color-ink);
  stroke: var(--color-ink);
}

.article-toc-rail__plane-shape {
  fill: var(--color-ink);
  stroke: var(--color-ink);
  stroke-width: 0.75;
  stroke-linejoin: round;
}
</style>
