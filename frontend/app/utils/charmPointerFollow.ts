/**
 * 指针端的"带阻尼跟随"（纯函数）。
 *
 * 为什么从 `decoCharm.ts` 拆出来：那个模块加完这段会顶到仓库的单文件 800 行门槛
 * （`frontend/eslint.config.mjs` 的 `max-lines`，与 `charmTether.ts` 当初拆出去是
 * 同一个理由）。拆的是文件，不是行为：依赖方向单向（`decoCharm.ts` → 本模块），
 * 本模块不持有状态、不认识链条，只吃三个点与一个刚度。
 *
 * 为什么指针端不能"每步硬赋值"：一帧里指针可以跳几十像素（60Hz 屏上 4000px/s
 * 就是 67px），硬赋值等于把这个位移当成瞬时冲量灌进链里，距离约束立刻把每节
 * 拉到静止长度 —— 看得见的就是"挂绳整条瞬直"。一阶跟随把同一段位移摊到多个
 * 固定子步上，链有子步级的反应时间，形状得以保留。
 */

/** 平面坐标（视口坐标系，与 `clientX / clientY` 同一套）。 */
export interface FollowPoint {
  x: number;
  y: number;
}

/**
 * 跟随的时间常数区间（s）：**由已有的 `stiffness` 换算，不新增配置**。
 *
 * 刚度越接近 1 跟得越紧、滞后越小；越软越"拖泥带水"。方向与 `stiffness` 的既有
 * 语义一致（软绳更容易被拖长、也更跟不紧），所以后台那一个旋钮同时管住了
 * "绳子多弹"和"指针端多黏"。
 *
 * 取值口径：τ 要让正常拖动速度（1000px/s 量级）的滞后只有几个像素、看起来只是
 * "绳头略显懒"，而不是永远贴着上界。默认刚度 0.5 换算出 τ ≈ 15ms。
 */
const FOLLOW_TAU_SOFT_SECONDS = 0.02;
/** 刚度上限（=1）时的跟随时间常数（s）：仍然留一点滞后，绝不会瞬移。 */
const FOLLOW_TAU_TIGHT_SECONDS = 0.006;

/**
 * 指针端离锚点的最大滞后（px）：**平滑不等于脱节**。
 *
 * 一阶跟随的稳态滞后是 `速度 × τ`，快速甩动时可以到上百像素 —— 那样指针端会
 * 明显飘在光标后面。这里给一个硬上界：超过它就把指针端按在圆周上。
 * 取 16px 是"看得出被拖着、但一眼就知道还连着"的量级（绳长 120px 的 13%）。
 */
export const CHARM_POINTER_FOLLOW_MAX_LAG_PX = 16;

/**
 * 吸附阈值（px）：间隙小于它就**精确**落到锚点上。
 *
 * 一阶跟随是指数逼近，永远差一点点；而静止姿态必须严格等于「指针 + 指针端偏移」
 * （ADR-0014 约束 7），否则渲染出来的指针端会长期浮在偏移之外。它同时是
 * `charmRopeSettled` 的第二个条件 —— 指针端还在往锚点爬的时候不许停帧。
 */
export const CHARM_POINTER_FOLLOW_SNAP_PX = 0.01;

/** 单步推进的固定时长（s）：与 `decoCharm.ts` 的物理固定步长保持一致。 */
const FOLLOW_STEP_SECONDS = 1 / 120;

/** 刚度区间（无量纲，0.2–1）：与 `decoCharm.ts` 的 `CHARM_STIFFNESS_MIN/MAX` 对齐。 */
const STIFFNESS_MIN = 0.2;
const STIFFNESS_MAX = 1;

/**
 * 每子步朝锚点靠多少比例（0–1）：由刚度换算的时间常数决定。
 *
 * `1 - e^(-dt/τ)` 是"一阶低通的精确离散解"，不是手调的近似 —— 所以同一段轨迹
 * 在 120Hz 与 60Hz 下积分出的姿态才一致（τ 是物理量，不是每帧比例）。
 */
export function pointerFollowAlpha(stiffness: number): number {
  const span = STIFFNESS_MAX - STIFFNESS_MIN;
  const safe = Number.isFinite(stiffness) ? Math.min(STIFFNESS_MAX, Math.max(STIFFNESS_MIN, stiffness)) : STIFFNESS_MAX;
  const ratio = span > 0 ? (safe - STIFFNESS_MIN) / span : 1;
  const tau = FOLLOW_TAU_SOFT_SECONDS + (FOLLOW_TAU_TIGHT_SECONDS - FOLLOW_TAU_SOFT_SECONDS) * ratio;
  return 1 - Math.exp(-FOLLOW_STEP_SECONDS / tau);
}

/**
 * 把指针端朝锚点推进一步（带阻尼、带滞后上界、带吸附），原地改写两个点。
 *
 * @param head 指针端节点的当前位置（会被改写）。
 * @param headPrevious 指针端节点的"上一步位置"（同步改写：它是被驱动的节点，
 *   没有残留速度可言；若把差值留在那里，下一子步的 Verlet 会把它当成速度重复计数）。
 * @param targetX 本子步的锚点 x（指针热点 + 指针端偏移，已插值）。
 * @param targetY 本子步的锚点 y。
 * @param stiffness 距离约束刚度，决定跟随的时间常数。
 */
export function followPointerAnchor(
  head: FollowPoint,
  headPrevious: FollowPoint,
  targetX: number,
  targetY: number,
  stiffness: number,
): void {
  const gapX = targetX - head.x;
  const gapY = targetY - head.y;
  const gap = Math.hypot(gapX, gapY);

  if (gap <= CHARM_POINTER_FOLLOW_SNAP_PX) {
    // 已经贴合：精确吸附，静止姿态严格等于「指针 + 偏移」。
    head.x = targetX;
    head.y = targetY;
  } else {
    const alpha = pointerFollowAlpha(stiffness);
    head.x += gapX * alpha;
    head.y += gapY * alpha;

    // 滞后上界：一阶跟随的稳态滞后是 `速度 × τ`，快速甩动时会过大。
    const restX = targetX - head.x;
    const restY = targetY - head.y;
    const rest = Math.hypot(restX, restY);
    if (rest > CHARM_POINTER_FOLLOW_MAX_LAG_PX) {
      head.x = targetX - (restX / rest) * CHARM_POINTER_FOLLOW_MAX_LAG_PX;
      head.y = targetY - (restY / rest) * CHARM_POINTER_FOLLOW_MAX_LAG_PX;
    }
  }

  headPrevious.x = head.x;
  headPrevious.y = head.y;
}
