/**
 * 光标挂件（cursor charm）的参数、门控与挂绳物理（纯函数）。
 *
 * 为什么单独成模块：挂件里所有"算错就看得见"的决定都在这里 —— 门控三条件、
 * 参数钳位、固定步长的 Verlet 链。它们全是纯函数，所以可以脱离浏览器逐点断言；
 * 组件与组合式只负责接线（事件、生命周期、停帧）。
 *
 * "绳点 → SVG 带形路径"那一层在 [`charmTether.ts`](./charmTether.ts)：它只吃
 * 一串点、吐一个 `d`，不持有状态，所以单独成文件（也让本模块留在单文件 800 行
 * 门槛内）。
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
 *     回落默认值，而不是钳到下限（**例外**是轻晃的两个参数与指针端偏移：
 *     它们的 0 有明确含义，走"0 合法"的规则）；
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
/** 静止摆动幅度默认值（px/s²），作用在自由端（逐节点加权，指针端为 0、尾端为 1）。 */
export const CHARM_IDLE_SWAY_AMP_DEFAULT = 20;

/**
 * 帧率上限的默认值与区间（fps）。
 *
 * 语义是**最高渲染帧率**，不是保证：高于显示器刷新率时等价于"每帧"
 * （160 档在 60Hz 屏上就等于不限，所以不需要再引入 `0 = 不限` 这种特殊值）。
 *
 * 下限为什么是 30：物理常量一律不动（1/120s 固定步长、0.05s 累加器上限），
 * 30 档一帧 33.3ms 对 0.05s 还留了 16.7ms 余量；20 档一帧恰好 50ms、余量为零，
 * 任何迟到都会被截断丢掉 —— 那会让绳子**慢放**，比"少画几次"糟得多。
 */
export const CHARM_FPS_CAP_MIN = 30;
/** 帧率上限的上限：再高在现实显示器上没有意义，只是让门控退化成"每帧"。 */
export const CHARM_FPS_CAP_MAX = 160;
/** 帧率上限默认值：与 60Hz 屏对齐（每帧一次）。 */
export const CHARM_FPS_CAP_DEFAULT = 60;

/**
 * 指针端偏移（票 #75）：每轴 -24–24 px、默认 3，**允许负值**。
 *
 * 语义是"物理头节点 = 指针热点 + (X, Y)"：默认 (3, 3) 让绳头从光标右下方
 * 悬挂，快速拖行时不再像"绳子从指尖里长出来"。负值表示往左上偏。
 *
 * 因为区间含负数，钳位不能用 `clampPositiveRange`（<= 0 回落默认）或
 * `clampNonNegativeRange`（负值夹回 min）：两者都会静默吃掉负偏移。ADR-0018
 * 决策 9 / 约束 8 为此新增第四条规则 `clampSignedRange`。
 */
export const CHARM_POINTER_OFFSET_X_MIN = -24;
/** 水平偏移上限（px）：再大绳头与光标热点明显脱节，看起来像"断线"。 */
export const CHARM_POINTER_OFFSET_X_MAX = 24;
/** 水平偏移默认值（px）：绳头从光标右侧 3px 处悬挂。 */
export const CHARM_POINTER_OFFSET_X_DEFAULT = 3;

/** 垂直偏移下限（px，视口坐标系里向下为正）：负数表示往上偏。 */
export const CHARM_POINTER_OFFSET_Y_MIN = -24;
/** 垂直偏移上限（px）。 */
export const CHARM_POINTER_OFFSET_Y_MAX = 24;
/** 垂直偏移默认值（px）：绳头从光标下方 3px 处悬挂。 */
export const CHARM_POINTER_OFFSET_Y_DEFAULT = 3;

/**
 * 门控的容差（ms）：判定写成 `now - lastPublishAt >= 1000 / fps_cap - 4`。
 *
 * 减 4ms 不是优化，是必需品：60Hz 屏上一帧是 16.6667ms，`1000/60` 也是
 * 16.6667ms，严格比较会因浮点与时间戳抖动**间歇性判成未到点**，把 60 档
 * 打成 30 档（ADR-0018 决策 3，不许删）。
 */
export const CHARM_FRAME_GATE_TOLERANCE_MS = 4;

/**
 * 一帧最早可以发布的间隔（ms）：`1000 / fps_cap - 容差`。
 *
 * 门控与测试共用这一个公式，避免"实现里减 4、测试里不减"这种假绿。
 */
export function charmFrameIntervalMs(fpsCap: number): number {
  return 1000 / clampCharmFpsCap(fpsCap) - CHARM_FRAME_GATE_TOLERANCE_MS;
}

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
  /** 静止摆动加速度幅度，px/s²，作用在自由端：逐节点加权（指针端 0、尾端 1）。0 表示关掉轻晃。 */
  idleSwayAmp: number;
  /** 最高渲染帧率，fps。只决定"每秒画几次"，不影响绳子运动速度（物理按真实时间推进）。 */
  fpsCap: number;
  /** 指针端偏移的水平分量，px，区间 -24–24（整数，允许负值）。物理头节点 = 指针热点 + 它。 */
  pointerOffsetX: number;
  /** 指针端偏移的垂直分量，px，区间 -24–24（整数，允许负值）。 */
  pointerOffsetY: number;
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
  /** 节点位置；nodes[0] 永远钉在**物理头节点**（指针热点 + 指针端偏移）上。 */
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

/**
 * 静止轻晃是否开启：幅度与频率**都必须大于 0**。
 *
 * 频率 0 表示"没有摆动"，此时正弦项恒为 0，物理是静止的 —— 若只看幅度，
 * 默认的幅度 20 会让组合式永远排着帧，白白耗电。两个 0 都按票面语义视为
 * "关掉轻晃"（收敛后即停帧）。
 */
export function charmIdleSwayEnabled(
  config: Pick<CharmRopeConfig, "idleSwayHz" | "idleSwayAmp">,
): boolean {
  return config.idleSwayAmp > 0 && config.idleSwayHz > 0;
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
 * 「0 与负数都是普通取值」的数值参数共用钳位规则 —— 第四条规则（票 #75）。
 *
 * 与另外两条的分界线只在"0 与负数怎么算"：
 *   - `clampPositiveRange`：<= 0 一律回落默认值；
 *   - `clampNonNegativeRange`：保留 0，但把负数夹回 min（这里的 min 是 -24，
 *     等于把"往左上偏 6px"静默改成"往左上偏 24px"）；
 *   - 这里：只夹到 [min, max]，0 与负数都不特殊对待。
 *
 * NaN / ±Inf 仍然回落默认值：它们序列化进 JSON 会直接失败。
 */
function clampSignedRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

/**
 * 取整成整数，半值远离 0（-2.5 → -3）。
 *
 * 为什么不直接用 `Math.round`：它对负半值往 +∞ 取整（-2.5 → -2），而后端用
 * Go 的 `math.Round`（远离 0，-2.5 → -3）。指针端偏移是**有符号**参数，负半值
 * 不是边角情形，跨端共享夹具里就钉着这一条。
 */
function roundHalfAwayFromZero(value: number): number {
  return Math.sign(value) * Math.round(Math.abs(value));
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
  // 与后端 `math.Round(v*10)/10` 同一条规则：toFixed 在半值上的舍入与
  // math.Round 不同（2.55 → "2.5" vs 2.6），跨端共享夹具会抓住这种漂移。
  return Math.round(clamped * 10) / 10;
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
 * 静止摆动幅度归一化：0 原样保留（= 关掉轻晃，收敛后停帧）；越界钳到 40，
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
 * 帧率上限归一化：越界钳到 30–160，零与非法值回落 60，最后取整成整数。
 *
 * 与后端 `normalizeFpsCap` 同一条规则（先夹边界、再 `math.Round`），
 * 顺序不能反：160.6 要先被钳到 160 再取整，否则会得到 161。
 */
export function clampCharmFpsCap(value: number): number {
  return Math.round(
    clampPositiveRange(value, CHARM_FPS_CAP_MIN, CHARM_FPS_CAP_MAX, CHARM_FPS_CAP_DEFAULT),
  );
}

/**
 * 指针端水平偏移归一化：夹到 -24–24 后取整成整数。
 *
 * 0 与负值都是普通取值（0 = 绳头压在热点上，负值 = 往左偏），只有 NaN / ±Inf
 * 回落默认 3。顺序是"先夹边界再取整"，与后端的 `normalizePointerOffset` 一致。
 */
export function clampCharmPointerOffsetX(value: number): number {
  return roundHalfAwayFromZero(
    clampSignedRange(
      value,
      CHARM_POINTER_OFFSET_X_MIN,
      CHARM_POINTER_OFFSET_X_MAX,
      CHARM_POINTER_OFFSET_X_DEFAULT,
    ),
  );
}

/** 指针端垂直偏移归一化：与水平分量完全对称（夹到 -24–24、取整、非法值回落 3）。 */
export function clampCharmPointerOffsetY(value: number): number {
  return roundHalfAwayFromZero(
    clampSignedRange(
      value,
      CHARM_POINTER_OFFSET_Y_MIN,
      CHARM_POINTER_OFFSET_Y_MAX,
      CHARM_POINTER_OFFSET_Y_DEFAULT,
    ),
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
    fpsCap: clampCharmFpsCap(Number(input?.fpsCap)),
    pointerOffsetX: clampCharmPointerOffsetX(Number(input?.pointerOffsetX)),
    pointerOffsetY: clampCharmPointerOffsetY(Number(input?.pointerOffsetY)),
  };
}

/** 把单个坐标收敛到有限值（非有限值一律当 0，避免 NaN 传进 SVG）。 */
function finite(value: number): number {
  return Number.isFinite(value) ? value : 0;
}

/**
 * 指针热点 + 偏移 = 物理头节点（票 #75 的唯一真相）。
 *
 * 为什么偏移必须在这一处换算：链的头节点被钉死在这里，弧垂与张力都从这个点
 * 算起。若改成"渲染时再把起点挪一下"，物理仍以光标为头，快速拉扯时起点与绳子
 * 会脱节（ADR-0018「已否决：偏移只挪渲染起点」）。所以渲染层**不得**再偏一次。
 *
 * 偏移在这里各自再钳一次：配置可能来自旧库、手改的 JSON 或别人直接调接口。
 */
export function charmPointerAnchor(
  pointer: CharmPoint,
  config: Pick<CharmRopeConfig, "pointerOffsetX" | "pointerOffsetY">,
): CharmPoint {
  return {
    x: finite(pointer.x) + clampCharmPointerOffsetX(Number(config?.pointerOffsetX)),
    y: finite(pointer.y) + clampCharmPointerOffsetY(Number(config?.pointerOffsetY)),
  };
}

/**
 * 建一条静止的链：节点 0 在物理头节点（指针热点 + 偏移）上，其余等距竖直下垂。
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

  // 静止轻晃：横向加速度按节点序号加权，**指针端为 0、自由端（尾节点）为 1**，
  // 按 `idle_sway_hz` 做正弦 —— 票面裁决要求加速度"作用在自由端"，所以越靠
  // 近指针施力越小、越靠尾端越大。指针端每步都被钉死，整条链因此是被距离
  // 约束"拉弯 / 侧摆"，而不是整层平移。幅度为 0 时这一项恒等于 0，物理回到
  // 纯重力 + 阻尼，可以正常收敛。
  const swayPhase = 2 * Math.PI * config.idleSwayHz * state.elapsedSeconds;
  const swayAccel = config.idleSwayAmp * Math.sin(swayPhase);
  const swayStep = swayAccel * CHARM_FIXED_STEP_SECONDS * CHARM_FIXED_STEP_SECONDS;

  for (let index = 1; index < count; index += 1) {
    const node = nodes[index];
    const previousNode = previous[index];
    if (!node || !previousNode) continue;
    const swayWeight = count > 1 ? index / (count - 1) : 0;
    const velocityX = (node.x - previousNode.x) * config.damping + swayStep * swayWeight;
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
