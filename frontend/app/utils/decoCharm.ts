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
 * 把任意来源的物理参数收敛成可以建链的形态。
 *
 * 四个字段各自独立钳位：某个键缺失或非法只影响它自己，不会把整份配置拖回默认。
 */
export function normalizeCharmRopeConfig(
  input: Partial<CharmRopeConfig> | null | undefined,
): CharmRopeConfig {
  return {
    ropeLengthPx: clampCharmRopeLength(Number(input?.ropeLengthPx)),
    segments: clampCharmSegments(Number(input?.segments)),
    gravity: clampCharmGravity(Number(input?.gravity)),
    damping: clampCharmDamping(Number(input?.damping)),
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
  for (let index = 1; index < count; index += 1) {
    const node = nodes[index];
    const previousNode = previous[index];
    if (!node || !previousNode) continue;
    const velocityX = (node.x - previousNode.x) * config.damping;
    const velocityY = (node.y - previousNode.y) * config.damping + gravityStep;
    previousNode.x = node.x;
    previousNode.y = node.y;
    node.x += velocityX;
    node.y += velocityY;
  }

  // 距离约束：每步松弛 segments 次（等于节点数），等效刚度 1。
  // 每次只把"超过静止长度的部分"对半分给两端；节点 0 钉死，所以第一段
  // 由节点 1 独自承担修正。
  const rest = state.restLengthPx;
  for (let iteration = 0; iteration < config.segments; iteration += 1) {
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
 * 挂绳路径（SVG 的 `d` 属性）：从指针依次连到每个自由节点。
 *
 * 本票是折线（`L`）；平滑曲线渲染属于票 #42，这里不提前做。
 * 坐标保留一位小数，避免每帧写出不同的长浮点。
 */
export function charmRopePath(state: CharmRopeState): string {
  const commands = state.nodes.map((node) => `${round1(node.x)} ${round1(node.y)}`);
  return `M${commands[0]}${commands.slice(1).map((command) => `L${command}`).join("")}`;
}
