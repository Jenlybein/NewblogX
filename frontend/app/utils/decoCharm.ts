/**
 * 光标挂件（cursor charm）的参数、门控与挂绳物理（纯函数）。
 *
 * 为什么单独成模块：挂件里所有"算错就看得见"的决定都在这里 —— 门控三条件、
 * 参数钳位、固定步长的 Verlet 链、绳点→SVG 路径。它们全是纯函数，所以可以
 * 脱离浏览器逐点断言；组件与组合式只负责接线（事件、生命周期、停帧）。
 *
 * 物理为什么是 Verlet 链而不是解析悬链线：解析悬链线零抖动，但站标会"死跟"
 * 指针，失去"被一根绳子拖着"的惯性感。指针端钉死、其余节点自由的链，
 * 静止姿态本来就是竖直的 —— **弧垂来自描边（绳长 > 实际距离），不来自重力**，
 * 所以调大重力只会让摆动更快，不会让绳子弯下来。
 *
 * 为什么是固定 1/120s 步长 + 累加器：被抄得最多的开源实现都硬编码"每帧一步"，
 * 在 120Hz 屏上物理会快一倍。这里按真实流逝时间累积，单帧最多 8 个子步、
 * 累加器上限 0.05s：掉帧、切后台、断点回来都不会让链被一帧甩爆。
 *
 * 四条坑：
 *   - 坐标可能是 NaN / Infinity（首帧、视口切换）：SVG 的 `d` 里出现 NaN 会
 *     让整条路径静默消失，所以一律收敛到 0；
 *   - 数值参数 0 是"配置字段没填"的零值，不是"我要最小"的意图 ——
 *     回落默认值，而不是钳到下限；
 *   - 距离约束在节点重合时 dx/dist 会炸，必须先做退化守卫；
 *   - 坐标保留一位小数：长浮点会让 `d` 字符串每帧都不同，白白制造 diff 噪声。
 */

/** 精确指针 + 可悬停设备的媒体查询：触屏与手写笔都匹配不上。 */
export const CHARM_FINE_POINTER_QUERY = "(hover: hover) and (pointer: fine)";

/** 挂绳长度下限（px）：再短就看不出"被绳子拖着"。与后端 `deco.charm.rope_length` 一致。 */
export const CHARM_ROPE_LENGTH_MIN = 40;
/** 挂绳长度上限（px）：再长会横穿阅读区。 */
export const CHARM_ROPE_LENGTH_MAX = 240;
/** 挂绳长度默认值（px）：与后端 `deco.charm.rope_length` 的默认值一致。 */
export const CHARM_ROPE_LENGTH_DEFAULT = 120;

/** 链节点数下限（节）：再少就不是绳子而是一根杆。与后端 `deco.charm.segments` 一致。 */
export const CHARM_SEGMENTS_MIN = 4;
/** 链节点数上限（节）：再多只是白烧 CPU。 */
export const CHARM_SEGMENTS_MAX = 24;
/** 链节点数默认值（节）。 */
export const CHARM_SEGMENTS_DEFAULT = 10;

/** 重力加速度下限（px/s²）。与后端 `deco.charm.gravity` 一致。 */
export const CHARM_GRAVITY_MIN = 400;
/** 重力加速度上限（px/s²）。 */
export const CHARM_GRAVITY_MAX = 4000;
/** 重力加速度默认值（px/s²）。 */
export const CHARM_GRAVITY_DEFAULT = 1600;

/** 每步速度保持系数下限（无量纲）。与后端 `deco.charm.damping` 一致。 */
export const CHARM_DAMPING_MIN = 0.94;
/** 每步速度保持系数上限：**必须小于 1**，否则每步都在放大速度，摆动会越甩越大。 */
export const CHARM_DAMPING_MAX = 0.995;
/** 每步速度保持系数默认值。 */
export const CHARM_DAMPING_DEFAULT = 0.99;

/** 站标尺寸下限（px）：再小就看不清是什么东西。与后端 `deco.charm.logo_size` 一致。 */
export const CHARM_LOGO_SIZE_MIN = 24;

/** 站标尺寸上限（px）：再大就会压住正文。 */
export const CHARM_LOGO_SIZE_MAX = 96;

/** 站标尺寸默认值（px）：与后端 `deco.charm.logo_size` 的默认值一致。 */
export const CHARM_LOGO_SIZE_DEFAULT = 48;

/** 描边起始宽度下限（px）：再细就看不见了。与后端 `deco.charm.line_width` 一致。 */
export const CHARM_LINE_WIDTH_MIN = 1;
/** 描边起始宽度上限（px）：再粗会抢正文。 */
export const CHARM_LINE_WIDTH_MAX = 6;
/** 描边起始宽度默认值（px）。 */
export const CHARM_LINE_WIDTH_DEFAULT = 2.5;

/**
 * 收尾宽度占起始宽度的比例。
 *
 * 票面把它钉成常数而不是配置项：它是"绳子有重量"的视觉签名，不是审美旋钮；
 * 再多一个参数只会让后台多一行没人看得懂的数字。
 */
export const CHARM_TAPER_RATIO = 0.6;

/** 静止摆动频率下限（Hz）。**0 是合法值**：它表示关掉轻晃。 */
export const CHARM_IDLE_SWAY_HZ_MIN = 0;
/** 静止摆动频率上限（Hz）。 */
export const CHARM_IDLE_SWAY_HZ_MAX = 1.5;
/** 静止摆动频率默认值（Hz）；0.8Hz 的周期是 1.25s。 */
export const CHARM_IDLE_SWAY_HZ_DEFAULT = 0.8;

/** 静止摆动幅度下限（px/s²）。**0 是合法值**：它让循环退回"静止即停帧"。 */
export const CHARM_IDLE_SWAY_AMP_MIN = 0;
/** 静止摆动幅度上限（px/s²）。 */
export const CHARM_IDLE_SWAY_AMP_MAX = 40;
/** 静止摆动幅度默认值（px/s²），作用在自由端。 */
export const CHARM_IDLE_SWAY_AMP_DEFAULT = 20;

/**
 * 怠速车道的一帧间隔（ms）。
 *
 * 轻晃的周期由 `idle_sway_hz` 决定（0.8Hz ≈ 1.25s），50ms 一帧 ≈ 20fps：
 * 对这条慢摆动足够平滑，又只有满速 60fps 的三分之一开销。裁决明确要求
 * "按节流后的帧率推进、每次只推进一个固定子步批次、不得补帧追赶"。
 */
export const CHARM_IDLE_FRAME_INTERVAL_MS = 50;

/**
 * 弧垂的几何系数。
 *
 * 松弛量与矢高不是线性关系：把绳子近似成一段圆弧 / 抛物线，弧长 L、弦长 c、
 * 矢高 s 满足 L − c ≈ (8/3) s² / c，所以 **s ≈ √(3 c (L − c) / 8)**。
 * 这解释了为什么"一点点松弛"就能看出一条明显的弧：松弛 0.1px 时矢高已有
 * 约 2px。松弛为 0 时矢高严格为 0（竖直悬挂就是直线）。
 */
const CHARM_SAG_NUMERATOR = 3;
const CHARM_SAG_DENOMINATOR = 8;

/**
 * 物理固定步长：1/120 s。
 *
 * 为什么不是"每帧一步"：60Hz 与 120Hz 屏幕上每帧的真实时长差一倍，
 * 硬编码每帧一步会让后者快一倍。固定步长 + 累加器让物理时间只由真实时间决定。
 */
export const CHARM_FIXED_STEP_SECONDS = 1 / 120;

/** 单帧最多推进的物理时间（s）：掉帧 / 断点回来时把累积差值截断在这里。 */
export const CHARM_MAX_FRAME_SECONDS = 0.05;

/** 单帧最多执行的子步数：与上面配合，防止一次巨大的 dt 让链被甩爆。 */
export const CHARM_MAX_SUBSTEPS = 8;

/**
 * 收敛阈值（px²/步²）。
 *
 * 能量按"每个自由节点一步内的位移平方和"计量（物理上等价于速度平方，
 * 只是少了除以 dt² 的常数）。10 个节点时 0.05 约等于每节点每步 0.07px ——
 * 在 120Hz 下路径字符串每帧变化不到一个像素，肉眼已经是静止。
 * 为什么不取更小：距离约束每步只松弛 `segments` 次，竖直链里重力每步
 * 被约束抵消后会留下一个极小的数值残差，取 0 会让循环多空转十几秒。
 */
export const CHARM_ENERGY_EPSILON = 0.05;

/** 挂件里的平面坐标（视口坐标系，与 `clientX / clientY` 同一套）。 */
export interface CharmPoint {
  x: number;
  y: number;
}

/** 启用判定的三个条件。 */
export interface CharmGateInput {
  /** `(hover: hover) and (pointer: fine)` 是否匹配。 */
  finePointerHover: boolean;
  /** 系统是否要求减少动态。 */
  prefersReducedMotion: boolean;
  /** 后台总开关。 */
  configEnabled: boolean;
}

/** 挂件的物理参数（与后端 `deco.charm` 对齐）。 */
export interface CharmRopeConfig {
  /** 挂绳总长（松垂阈值），px。 */
  ropeLengthPx: number;
  /** Verlet 链节点数（节）。 */
  segments: number;
  /** 重力加速度，px/s²。 */
  gravity: number;
  /** 每步速度保持系数，无量纲。 */
  damping: number;
  /** 描边起始宽度，px（收尾固定为它的 0.6 倍）。 */
  lineWidthPx: number;
  /** 静止摆动频率，Hz。0 表示关掉轻晃。 */
  idleSwayHz: number;
  /** 静止摆动加速度幅度，px/s²，作用在自由端。0 表示关掉轻晃。 */
  idleSwayAmp: number;
}

/**
 * 挂绳的物理状态：**非响应式普通对象**。
 *
 * 为什么故意不用 Vue 的 ref/reactive：它每秒会被改写 120 次，
 * 让响应式系统逐帧追踪十个节点的深比较纯属白烧 CPU。
 * 每帧只有路径字符串与一个 transform 需要对外可见。
 */
export interface CharmRopeState {
  /** 建链时用的参数快照（节点数 / 绳长变化时需要重建）。 */
  config: CharmRopeConfig;
  /** 每节的静止长度（px），= ropeLength / segments。 */
  restLengthPx: number;
  /** 节点位置；nodes[0] 永远钉在指针上。 */
  nodes: CharmPoint[];
  /** 上一步的位置，Verlet 用它算速度。 */
  previous: CharmPoint[];
  /** 还没凑满一个固定步长的真实时间（s）。 */
  accumulatorSeconds: number;
  /** 物理世界已经流逝的时间（s）：轻晃的正弦相位按它取值，与帧率无关。 */
  elapsedSeconds: number;
  /** 已经执行过的固定子步总数（测试与调试用）。 */
  stepCount: number;
}

/**
 * 挂件层是否启用：精确指针 + 未要求减少动态 + 后台总开关。
 *
 * 触屏设备上没有"悬停"这回事，跟着手指的绳子毫无意义；「减少动态」要求的是
 * 整站不出现，而不是放慢。两个都满足时才谈得上渲染。
 */
export function charmLayerEnabled(input: CharmGateInput): boolean {
  return input.finePointerHover && !input.prefersReducedMotion && input.configEnabled;
}

/** 数值参数共用的钳位规则：越界钳到边界、非有限与零值回落默认。 */
function clampPositiveRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value <= 0) return fallback;
  return Math.min(max, Math.max(min, value));
}

/** 「0 有合法含义」的数值参数共用钳位规则（静止摆动频率 / 幅度）。 */
function clampNonNegativeRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 0) return fallback;
  return Math.min(max, Math.max(min, value));
}

/**
 * 站标尺寸归一化：越界钳到边界、非正值与非法值回落默认值。
 *
 * 后端在写入路径上已经钳过一次，这里是最后一道防线（配置可能来自旧库、
 * 手改的 JSON、或者别人直接调接口）。
 */
export function clampCharmLogoSize(value: number): number {
  return clampPositiveRange(value, CHARM_LOGO_SIZE_MIN, CHARM_LOGO_SIZE_MAX, CHARM_LOGO_SIZE_DEFAULT);
}

/** 挂绳长度归一化：越界钳到 40–240，零与非法值回落 120。 */
export function clampCharmRopeLength(value: number): number {
  return clampPositiveRange(value, CHARM_ROPE_LENGTH_MIN, CHARM_ROPE_LENGTH_MAX, CHARM_ROPE_LENGTH_DEFAULT);
}

/**
 * 节点数归一化：计数先四舍五入，再钳到 4–24，零与非法值回落 10。
 *
 * 四舍五入而不是截断是有意的：管理员填 10.6 的意图更接近 11（更顺滑），
 * 而"10.6 节"本身在节点数组里没有意义。
 */
export function clampCharmSegments(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return CHARM_SEGMENTS_DEFAULT;
  return Math.min(CHARM_SEGMENTS_MAX, Math.max(CHARM_SEGMENTS_MIN, Math.round(value)));
}

/** 重力归一化：越界钳到 400–4000，零与非法值回落 1600。 */
export function clampCharmGravity(value: number): number {
  return clampPositiveRange(value, CHARM_GRAVITY_MIN, CHARM_GRAVITY_MAX, CHARM_GRAVITY_DEFAULT);
}

/** 阻尼归一化：越界钳到 0.94–0.995（上限小于 1），零与非法值回落 0.99。 */
export function clampCharmDamping(value: number): number {
  return clampPositiveRange(value, CHARM_DAMPING_MIN, CHARM_DAMPING_MAX, CHARM_DAMPING_DEFAULT);
}

/**
 * 描边起始宽度归一化：越界钳到 1–6，零与非法值回落 2.5，并保留一位小数。
 *
 * 保留一位小数是因为后台输入框会回填这个值：2.5400000000000002 这种长浮点
 * 会让人觉得"不是我填的数"。
 */
export function clampCharmLineWidth(value: number): number {
  const clamped = clampPositiveRange(
    value,
    CHARM_LINE_WIDTH_MIN,
    CHARM_LINE_WIDTH_MAX,
    CHARM_LINE_WIDTH_DEFAULT,
  );
  return Number(clamped.toFixed(1));
}

/**
 * 静止摆动频率归一化：0 原样保留（= 关掉轻晃）；越界钳到 1.5，
 * 负数与非法值回落 0.8。
 */
export function clampCharmIdleSwayHz(value: number): number {
  return clampNonNegativeRange(
    value,
    CHARM_IDLE_SWAY_HZ_MIN,
    CHARM_IDLE_SWAY_HZ_MAX,
    CHARM_IDLE_SWAY_HZ_DEFAULT,
  );
}

/**
 * 静止摆动幅度归一化：0 原样保留（= 退回零帧车道）；越界钳到 40，
 * 负数与非法值回落 20。
 */
export function clampCharmIdleSwayAmp(value: number): number {
  return clampNonNegativeRange(
    value,
    CHARM_IDLE_SWAY_AMP_MIN,
    CHARM_IDLE_SWAY_AMP_MAX,
    CHARM_IDLE_SWAY_AMP_DEFAULT,
  );
}

/**
 * 把任意来源的物理参数收敛成可以建链的形态。
 *
 * 每个字段各自独立钳位：某个键缺失或非法只影响它自己，不会把整份配置拖回默认。
 */
export function normalizeCharmRopeConfig(
  input: Partial<CharmRopeConfig> | null | undefined,
): CharmRopeConfig {
  return {
    ropeLengthPx: clampCharmRopeLength(Number(input?.ropeLengthPx)),
    segments: clampCharmSegments(Number(input?.segments)),
    gravity: clampCharmGravity(Number(input?.gravity)),
    damping: clampCharmDamping(Number(input?.damping)),
    lineWidthPx: clampCharmLineWidth(Number(input?.lineWidthPx)),
    idleSwayHz: clampCharmIdleSwayHz(Number(input?.idleSwayHz)),
    idleSwayAmp: clampCharmIdleSwayAmp(Number(input?.idleSwayAmp)),
  };
}

/** 把单个坐标收敛到有限值（非有限值一律当 0，避免 NaN 传进 SVG）。 */
function finite(value: number): number {
  return Number.isFinite(value) ? value : 0;
}

/** 保留一位小数，避免长浮点每帧写出不同的 `d` 字符串。 */
function round1(value: number): number {
  return Number(finite(value).toFixed(1));
}

/**
 * 建一条静止的链：节点 0 在指针上，其余等距竖直下垂。
 *
 * 初始是"竖直静止"而不是"从指针处拖一根弹簧"：这样第一帧的能量为零，
 * 不会在页面刚打开时先甩一下。真正的摆动从指针第一次移动开始。
 */
export function createCharmRope(head: CharmPoint, config: CharmRopeConfig): CharmRopeState {
  const normalized = normalizeCharmRopeConfig(config);
  const restLengthPx = normalized.ropeLengthPx / normalized.segments;
  const headPoint = { x: finite(head.x), y: finite(head.y) };
  const nodes: CharmPoint[] = [];
  for (let index = 0; index <= normalized.segments; index += 1) {
    nodes.push({ x: headPoint.x, y: headPoint.y + restLengthPx * index });
  }
  return {
    config: normalized,
    restLengthPx,
    nodes,
    previous: nodes.map((node) => ({ ...node })),
    accumulatorSeconds: 0,
    elapsedSeconds: 0,
    stepCount: 0,
  };
}

/** 一次固定步长的 Verlet 推进（含约束松弛）。 */
function substep(state: CharmRopeState, head: CharmPoint): void {
  const { nodes, previous, config } = state;
  const first = nodes[0];
  const firstPrevious = previous[0];
  if (!first || !firstPrevious) return;
  const headX = finite(head.x);
  const headY = finite(head.y);
  const count = nodes.length;

  // 指针端每步都钉死在当前位置：它不是被力推着走，而是"手的位置"。
  first.x = headX;
  first.y = headY;
  firstPrevious.x = headX;
  firstPrevious.y = headY;

  const gravityStep = config.gravity * CHARM_FIXED_STEP_SECONDS * CHARM_FIXED_STEP_SECONDS;

  // 静止轻晃：自由端的横向加速度，按 `idle_sway_hz` 做正弦。权重随节点
  // 序号增大（尾端最大），所以它把链"拉弯"，而不是整层平移 —— 指针端
  // 每步都被钉死，平移在这里没有意义。幅度为 0 时这一项恒等于 0，
  // 物理就回到纯重力 + 阻尼，可以正常收敛。
  const swayPhase = 2 * Math.PI * config.idleSwayHz * state.elapsedSeconds;
  const swayAccel = config.idleSwayAmp * Math.sin(swayPhase);
  const swayStep = swayAccel * CHARM_FIXED_STEP_SECONDS * CHARM_FIXED_STEP_SECONDS;

  for (let index = 1; index < count; index += 1) {
    const node = nodes[index];
    const previousNode = previous[index];
    if (!node || !previousNode) continue;
    // 每个自由节点都感受到同样的横向加速度；指针端被钉死，所以整条链是
    // "被拉弯 / 侧摆"，而不是整层平移。幅度为 0 时这一项恒为 0，物理回到
    // 纯重力 + 阻尼，可以正常收敛到停。
    const velocityX = (node.x - previousNode.x) * config.damping + swayStep;
    const velocityY = (node.y - previousNode.y) * config.damping + gravityStep;
    previousNode.x = node.x;
    previousNode.y = node.y;
    node.x += velocityX;
    node.y += velocityY;
  }
  state.elapsedSeconds += CHARM_FIXED_STEP_SECONDS;

  // 距离约束：每步松弛 2 × segments 次（等效刚度更高）。
  // 为什么是 2 倍：一次松弛留下的每节误差会累积成"弦长比绳长还长"的
  // 数值假象（10 节可累到 0.4px），而弧垂是从"绳长 − 弦长"算出来的，
  // 这点误差会把真实松弛吃掉。翻倍迭代把残差压到 ~0.01px，代价只有
  // 每子步多算几遍（10 节点 × 20 次，120Hz 下仍然可以忽略）。
  // 每次只把"超过静止长度的部分"对半分给两端；节点 0 钉死，所以第一段
  // 由节点 1 独自承担修正。
  const rest = state.restLengthPx;
  const iterationCount = config.segments * 30;
  for (let iteration = 0; iteration < iterationCount; iteration += 1) {
    first.x = headX;
    first.y = headY;
    for (let index = 0; index < count - 1; index += 1) {
      const current = nodes[index];
      const next = nodes[index + 1];
      if (!current || !next) continue;
      const deltaX = next.x - current.x;
      const deltaY = next.y - current.y;
      const distance = Math.hypot(deltaX, deltaY);

      // 退化守卫：两节点重合时 dx/dist 会炸成 Infinity。
      // 把它们沿竖直方向拉开一个静止长度（这是唯一会"瞬移"的路径，
      // 只可能发生在极端数值下，正常甩动到不了这里）。
      if (!(distance > 1e-6)) {
        next.x = current.x;
        next.y = current.y + rest;
        continue;
      }

      const correction = (distance - rest) / distance;
      if (index === 0) {
        // 指针端是钉死的，整段修正都给节点 1。
        next.x -= deltaX * correction;
        next.y -= deltaY * correction;
      } else {
        current.x += (deltaX * correction) / 2;
        current.y += (deltaY * correction) / 2;
        next.x -= (deltaX * correction) / 2;
        next.y -= (deltaY * correction) / 2;
      }
    }
  }
  first.x = headX;
  first.y = headY;
}

/**
 * 推进一帧的真实时间：把 dt 塞进累加器，再按固定步长跑子步。
 *
 * @returns 本帧真正执行的子步数（测试用来断言"没有比真实时间跑得快"）。
 *
 * 累加器先截断到 `CHARM_MAX_FRAME_SECONDS`：切后台十分钟再回来、
 * 断点停在某一行，都不会让链在一帧里被甩爆；单帧子步上限是第二道保险。
 */
export function advanceCharmRope(state: CharmRopeState, frameDeltaSeconds: number, head: CharmPoint): number {
  if (!Number.isFinite(frameDeltaSeconds) || frameDeltaSeconds <= 0) return 0;

  state.accumulatorSeconds = Math.min(
    CHARM_MAX_FRAME_SECONDS,
    state.accumulatorSeconds + frameDeltaSeconds,
  );

  let subSteps = 0;
  while (state.accumulatorSeconds >= CHARM_FIXED_STEP_SECONDS && subSteps < CHARM_MAX_SUBSTEPS) {
    substep(state, head);
    state.accumulatorSeconds -= CHARM_FIXED_STEP_SECONDS;
    state.stepCount += 1;
    subSteps += 1;
  }
  // 打到子步上限时丢弃剩下的时间，而不是留到下一帧继续补 ——
  // 否则重负载下会进入"永远在追赶"的正反馈。
  if (subSteps >= CHARM_MAX_SUBSTEPS && state.accumulatorSeconds >= CHARM_FIXED_STEP_SECONDS) {
    state.accumulatorSeconds = 0;
  }
  return subSteps;
}

/** 当前链的能量（px²/步²）：每个自由节点一步内位移的平方和。 */
export function charmRopeEnergy(state: CharmRopeState): number {
  let energy = 0;
  for (let index = 1; index < state.nodes.length; index += 1) {
    const node = state.nodes[index];
    const previousNode = state.previous[index];
    if (!node || !previousNode) continue;
    const deltaX = node.x - previousNode.x;
    const deltaY = node.y - previousNode.y;
    energy += deltaX * deltaX + deltaY * deltaY;
  }
  return energy;
}

/**
 * 链是否已经收敛到静止：能量低于阈值。
 *
 * 组合式据此取消动画帧 —— 静态页面必须零帧开销，不能永远空转。
 */
export function charmRopeSettled(state: CharmRopeState, threshold: number = CHARM_ENERGY_EPSILON): boolean {
  return charmRopeEnergy(state) < threshold;
}

/** 除指针端以外的最后一个节点（站标挂在这里）。 */
export function charmRopeTail(state: CharmRopeState): CharmPoint {
  const tail = state.nodes[state.nodes.length - 1];
  return tail ? { ...tail } : { x: 0, y: 0 };
}

/** 每节的当前长度（测试用来断言约束容差与链长守恒）。 */
export function charmRopeSegmentLengths(state: CharmRopeState): number[] {
  const lengths: number[] = [];
  for (let index = 0; index < state.nodes.length - 1; index += 1) {
    const current = state.nodes[index];
    const next = state.nodes[index + 1];
    if (!current || !next) continue;
    const deltaX = next.x - current.x;
    const deltaY = next.y - current.y;
    lengths.push(Math.hypot(deltaX, deltaY));
  }
  return lengths;
}

/**
 * 中心线的采样密度：每个链节点区间取 4 个 Catmull-Rom 采样点。
 *
 * 4 段/区间已经让 10 节点的折线在 1px 级看不出棱角；再加密只是让每帧的
 * `d` 字符串变长，视觉收益为零。
 */
const CHARM_CURVE_SAMPLES_PER_SEGMENT = 4;

/** 挂绳的几何结果：中心线、每点宽度、松弛量、弧垂与可直接写进 SVG 的带形路径。 */
export interface CharmTetherGeometry {
  /** 平滑中心线（经过链节点；原始精度，不取整）。 */
  centerline: CharmPoint[];
  /** 每个中心线采样点处的描边宽度（px）；首项 = line_width，末项 = 0.6 × line_width。 */
  widths: number[];
  /** 松驰量（px）= max(0, 绳长 − 指针到站标的实际距离)。 */
  slackPx: number;
  /** 中心线相对头尾弦的最大横向偏移（px）：这就是肉眼看到的"弧垂"。 */
  maxDeviationPx: number;
  /** 闭合的带形多边形（SVG `<path d>` 用 `fill`，不是 `stroke`）。 */
  path: string;
}

/** 单个 Catmull-Rom 采样点。 */
function catmullRomPoint(p0: CharmPoint, p1: CharmPoint, p2: CharmPoint, p3: CharmPoint, t: number): CharmPoint {
  const t2 = t * t;
  const t3 = t2 * t;
  return {
    x:
      0.5 *
      (2 * p1.x +
        (-p0.x + p2.x) * t +
        (2 * p0.x - 5 * p1.x + 4 * p2.x - p3.x) * t2 +
        (-p0.x + 3 * p1.x - 3 * p2.x + p3.x) * t3),
    y:
      0.5 *
      (2 * p1.y +
        (-p0.y + p2.y) * t +
        (2 * p0.y - 5 * p1.y + 4 * p2.y - p3.y) * t2 +
        (-p0.y + 3 * p1.y - 3 * p2.y + p3.y) * t3),
  };
}

/**
 * 把链节点插值成一条经过每个节点的平滑折线（Catmull-Rom）。
 *
 * 为什么要插值而不直接连节点：10 个节点的折线在甩动时会露出棱角，
 * 而"有重量的绳子"读起来必须是连续的。采样点保留原始精度，取整只发生在
 * 写进 `d` 的那一刻。
 */
function smoothCenterline(points: readonly CharmPoint[]): CharmPoint[] {
  const count = points.length;
  if (count === 0) return [];
  if (count === 1) return [{ ...points[0]! }];
  const samples: CharmPoint[] = [];
  for (let index = 0; index < count - 1; index += 1) {
    const p0 = points[Math.max(0, index - 1)]!;
    const p1 = points[index]!;
    const p2 = points[index + 1]!;
    const p3 = points[Math.min(count - 1, index + 2)]!;
    for (let sample = 0; sample < CHARM_CURVE_SAMPLES_PER_SEGMENT; sample += 1) {
      samples.push(catmullRomPoint(p0, p1, p2, p3, sample / CHARM_CURVE_SAMPLES_PER_SEGMENT));
    }
  }
  samples.push({ ...points[count - 1]! });
  return samples;
}

/**
 * 挂绳的绘制几何。
 *
 * 三件事在这里同时决定，所以单独成一个纯函数、供测试直接喂"几何链"：
 *
 * 1. **弧垂只由松弛驱动，绝不来自重力**。松弛 = 绳长 − 指针到站标的实际
 *    距离；两端距离恰好等于绳长时矢高为 0（竖直悬挂就是一条直线），
 *    松弛越大中段偏离弦越多。重力只影响链的瞬态姿态（节点的实际位置），
 *    从不进入这个函数 —— 调大重力不会让静止的绳子弯下来。
 * 2. **收尾描边恰好是起始的 0.6 倍**。一根 `stroke` 变不了宽度，所以这里
 *    生成一条**填充的带形多边形**：两侧边界沿中心线法向各偏移半个宽度，
 *    宽度从 `line_width` 线性收到 `0.6 × line_width`。每帧仍然只写一个
 *    `d`，不增加 DOM 写入次数。
 * 3. 坐标保留一位小数，避免每帧写出长浮点制造 diff 噪声。
 */
export function charmTetherGeometry(
  points: readonly CharmPoint[],
  ropeLengthPx: number,
  lineWidthPx: number,
): CharmTetherGeometry {
  const widthStart = clampCharmLineWidth(lineWidthPx);
  const ropeLength = Number.isFinite(ropeLengthPx) && ropeLengthPx > 0 ? ropeLengthPx : 0;
  const centerline = smoothCenterline(points);
  const count = centerline.length;
  if (count === 0) {
    return { centerline: [], widths: [], slackPx: 0, maxDeviationPx: 0, path: "" };
  }

  // 两端之间的弦：它是"实际距离"，弧垂的量尺。
  const head = centerline[0]!;
  const tail = centerline[count - 1]!;
  const chordX = tail.x - head.x;
  const chordY = tail.y - head.y;
  const chord = Math.hypot(chordX, chordY);
  const slackPx = Math.max(0, ropeLength - chord);

  // 弧的方向：顺着链自己的横向偏移（物理形变的方向）；纯直线的合成几何
  // 没有方向可说，就取弦的左侧作为确定性默认。两端点处权重为 0。
  const normalX = chord > 1e-9 ? -chordY / chord : 0;
  const normalY = chord > 1e-9 ? chordX / chord : 1;
  let side = 0;
  for (const point of centerline) {
    side += (point.x - head.x) * normalX + (point.y - head.y) * normalY;
  }
  const direction = side < 0 ? -1 : 1;
  // 松弛 → 矢高：s ≈ √(3 c slack / 8)。两端恰好等于绳长时 slack = 0 → 直线。
  const sagPeakPx = Math.sqrt((CHARM_SAG_NUMERATOR * chord * slackPx) / CHARM_SAG_DENOMINATOR);

  // 宽度与弧垂都按"沿链的归一化参数"插值：首尾宽度固定，中段弧垂最大。
  const widths: number[] = [];
  const offsetSamples: CharmPoint[] = [];
  for (let index = 0; index < count; index += 1) {
    const u = count > 1 ? index / (count - 1) : 1;
    const point = centerline[index]!;
    const bowPx = sagPeakPx * 4 * u * (1 - u);
    const bowed = {
      x: point.x + normalX * direction * bowPx,
      y: point.y + normalY * direction * bowPx,
    };
    centerline[index] = bowed;
    offsetSamples.push(bowed);
    widths.push(widthStart * (1 - (1 - CHARM_TAPER_RATIO) * u));
  }

  // 弧垂 = 中心线相对弦的最大横向偏移（用同一把尺子量，测试与显示一致）。
  let maxDeviationPx = 0;
  const chordUnitX = chord > 1e-9 ? chordX / chord : 0;
  const chordUnitY = chord > 1e-9 ? chordY / chord : 1;
  for (const point of centerline) {
    const dx = point.x - head.x;
    const dy = point.y - head.y;
    maxDeviationPx = Math.max(maxDeviationPx, Math.abs(dx * chordUnitY - dy * chordUnitX));
  }

  // 带形多边形：中心线两侧各偏移半个宽度。两侧各走一遍，尾端用一条
  // `L` 连起来，首端由 `Z` 闭合。
  const left: CharmPoint[] = [];
  const right: CharmPoint[] = [];
  for (let index = 0; index < count; index += 1) {
    const point = centerline[index]!;
    const previous = centerline[Math.max(0, index - 1)]!;
    const next = centerline[Math.min(count - 1, index + 1)]!;
    const tangentX = next.x - previous.x;
    const tangentY = next.y - previous.y;
    const tangentLength = Math.hypot(tangentX, tangentY);
    const unitX = tangentLength > 1e-9 ? tangentX / tangentLength : 0;
    const unitY = tangentLength > 1e-9 ? tangentY / tangentLength : 1;
    const half = widths[index]! / 2;
    const nx = -unitY * half;
    const ny = unitX * half;
    left.push({ x: point.x + nx, y: point.y + ny });
    right.push({ x: point.x - nx, y: point.y - ny });
  }

  const leftCommands = left.slice(1).map((point) => `L${round1(point.x)} ${round1(point.y)}`);
  const rightCommands = right
    .slice(0, -1)
    .reverse()
    .map((point) => `L${round1(point.x)} ${round1(point.y)}`);
  const path =
    `M${round1(left[0]!.x)} ${round1(left[0]!.y)}` +
    leftCommands.join("") +
    `L${round1(right[right.length - 1]!.x)} ${round1(right[right.length - 1]!.y)}` +
    rightCommands.join("") +
    "Z";

  return { centerline, widths, slackPx, maxDeviationPx, path };
}

/**
 * 挂绳的 SVG `d`：闭合的填充带形多边形（不是一根等宽 `stroke`）。
 *
 * 单根 SVG `stroke` 无法变宽，所以用"沿中心线两侧偏移半宽"的带形多边形来
 * 表达收尾（起始 → 0.6 倍）。好处是每帧仍然只有一个 `d` 写入；代价是
 * `d` 比折线长一些（采样点 × 2 条边），不碰任何布局属性。
 */
export function charmRopePath(state: CharmRopeState): string {
  return charmTetherGeometry(state.nodes, state.config.ropeLengthPx, state.config.lineWidthPx).path;
}
