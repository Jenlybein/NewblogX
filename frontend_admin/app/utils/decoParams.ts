/**
 * 装饰（光标挂件）参数的元数据与钳位。
 *
 * 为什么放在 `app/utils`：后台的四道覆盖率门槛只统计 `app/utils/**`
 * （`vitest.config.ts` 的 `coverage.include`），把标签 / 单位 / 范围 / 默认值
 * 放在这一层，它们天然被门槛盯着；表单组件只消费这里，不自己写数字。
 *
 * 为什么末尾要再钳一次：`NInputNumber` 的 `:min/:max` 只是 UI 约束 ——
 * 粘贴、浏览器 devtools、旧版本页面都能绕过它。后端写入路径已经钳过，
 * 这里是同一规则的第二道防线（票面要求"两端各做一次同样的钳位"），
 * 而且**不报错**：管理员改一半被 400 打回只会让他猜哪里错了。
 *
 * 取值必须与后端 `backend/conf/conf_deco.go` 一致。跨端一致性门禁是票 #43 的范围，
 * 本票只在单测里显式钉住这些数，改一端会先在这里红一次。
 */

/** 站标尺寸下限（px）。 */
export const DECO_LOGO_SIZE_MIN = 15

/** 站标尺寸上限（px）。 */
export const DECO_LOGO_SIZE_MAX = 70

/** 站标尺寸默认值（px）。 */
export const DECO_LOGO_SIZE_DEFAULT = 48

/** 挂绳长度下限（px）。 */
export const DECO_ROPE_LENGTH_MIN = 40

/** 挂绳长度上限（px）。 */
export const DECO_ROPE_LENGTH_MAX = 240

/** 挂绳长度默认值（px）。 */
export const DECO_ROPE_LENGTH_DEFAULT = 120

/** 链节点数下限（节）。 */
export const DECO_SEGMENTS_MIN = 4

/** 链节点数上限（节）。 */
export const DECO_SEGMENTS_MAX = 30

/** 链节点数默认值（节）。 */
export const DECO_SEGMENTS_DEFAULT = 10

/** 重力加速度下限（px/s²）。 */
export const DECO_GRAVITY_MIN = 400

/** 重力加速度上限（px/s²）。 */
export const DECO_GRAVITY_MAX = 4000

/** 重力加速度默认值（px/s²）。 */
export const DECO_GRAVITY_DEFAULT = 1600

/** 每步速度保持系数下限（无量纲）。 */
export const DECO_DAMPING_MIN = 0.9

/**
 * 每步速度保持系数上限（无量纲）。
 *
 * 1 表示速度完全不衰减：摆动会靠重力继续来回，可能持续振荡（已接受的风险）。
 * 大于 1 才会逐步放大速度、越甩越大，所以越界值仍然钳回 1。
 */
export const DECO_DAMPING_MAX = 1

/** 每步速度保持系数默认值（无量纲）。 */
export const DECO_DAMPING_DEFAULT = 0.99

/** 距离约束刚度下限（无量纲，0–1）：再软就是橡皮筋，拖动后久久回不到位。 */
export const DECO_STIFFNESS_MIN = 0.2

/** 距离约束刚度上限：1 = 每次迭代全量投影，手感回到"刚杆"。 */
export const DECO_STIFFNESS_MAX = 1

/** 距离约束刚度默认值（无量纲）：一次投影只消化一半误差，拖动时先拉长再回弹。 */
export const DECO_STIFFNESS_DEFAULT = 0.5

/** 描边起始宽度下限（px）。 */
export const DECO_LINE_WIDTH_MIN = 1

/** 描边起始宽度上限（px）。 */
export const DECO_LINE_WIDTH_MAX = 6

/** 描边起始宽度默认值（px）；收尾固定为它的 0.6 倍（不是配置项）。 */
export const DECO_LINE_WIDTH_DEFAULT = 2.5

/** 静止摆动频率下限（Hz）。0 是合法值：表示关掉轻晃。 */
export const DECO_IDLE_SWAY_HZ_MIN = 0

/** 静止摆动频率上限（Hz）。 */
export const DECO_IDLE_SWAY_HZ_MAX = 1.5

/** 静止摆动频率默认值（Hz）。 */
export const DECO_IDLE_SWAY_HZ_DEFAULT = 0.8

/** 静止摆动幅度下限（px/s²）。0 是合法值：退回"静止即零帧"。 */
export const DECO_IDLE_SWAY_AMP_MIN = 0

/** 静止摆动幅度上限（px/s²）。 */
export const DECO_IDLE_SWAY_AMP_MAX = 40

/** 静止摆动幅度默认值（px/s²），作用在自由端（逐节点加权，指针端节点为 0、尾端为 1）。 */
export const DECO_IDLE_SWAY_AMP_DEFAULT = 20

/** 帧率上限下限（fps）。30 档一帧 33.3ms，仍在物理累加器 0.05s 的余量内。 */
export const DECO_FPS_CAP_MIN = 30

/** 帧率上限上限（fps）。高于显示器刷新率时等价于每帧，所以不需要"0 = 不限"。 */
export const DECO_FPS_CAP_MAX = 160

/** 帧率上限默认值（fps）：与 60Hz 屏对齐（每帧一次）。 */
export const DECO_FPS_CAP_DEFAULT = 60

/** 指针端偏移水平分量下限（px）。**负数是合法值**：指针端挂到光标左侧。 */
export const DECO_POINTER_OFFSET_X_MIN = -24

/** 指针端偏移水平分量上限（px）：再大指针端与光标热点明显脱节。 */
export const DECO_POINTER_OFFSET_X_MAX = 24

/** 指针端偏移水平分量默认值（px）：指针端从光标右侧 3px 处悬挂。 */
export const DECO_POINTER_OFFSET_X_DEFAULT = 3

/** 指针端偏移垂直分量下限（px，向下为正）：负数表示往上偏。 */
export const DECO_POINTER_OFFSET_Y_MIN = -24

/** 指针端偏移垂直分量上限（px）。 */
export const DECO_POINTER_OFFSET_Y_MAX = 24

/** 指针端偏移垂直分量默认值（px）：指针端从光标下方 3px 处悬挂。 */
export const DECO_POINTER_OFFSET_Y_DEFAULT = 3

/**
 * 一个数值参数用哪条钳位规则（票 #78）—— 元数据驱动归一化的关键。
 *
 * 五个取值与后端 `Normalize()` 的规则一一对应：
 *   - `positive`：<= 0 是"没填 / 手滑"，回落默认值（站标尺寸、挂绳长度、重力、阻尼、刚度）；
 *   - `nonNegative`：0 有合法含义（关掉轻晃），负数才回落默认；
 *   - `signed`：0 与负数都是普通取值，只夹 [min, max] 再取整（指针端偏移）；
 *   - `segments`：整数计数（`segments` / `fps_cap`），夹边界后取整；
 *   - `lineWidth`：正数规则之后再收成 0.1 的整数倍（描边宽度）。
 */
export type DecoParamKind = 'positive' | 'nonNegative' | 'signed' | 'segments' | 'lineWidth'

/** 一个数值参数的元数据；表单、默认值、归一化与守卫测试都只认这个形状。 */
export interface DecoNumberParamMeta {
  /**
   * 提交给后端的键名（snake_case，与 Go 的 json tag 一致）。
   *
   * 这里故意只声明成 `string`：具体键名由每个参数对象经 `defineDecoParam`
   * 钉成字面量，再由 `DECO_NUMBER_PARAMS` 派生 `DecoNumberParamKey` ——
   * 键名清单只有一份，不从接口里抄第二份。
   */
  key: string
  /** 表单项标签。 */
  label: string
  /** 单位后缀，直接显示在输入框右侧。 */
  unit: string
  /** 一句话说明：这个参数影响什么。 */
  hint: string
  /** 归一化用哪条钳位规则（见 `DecoParamKind`）。 */
  kind: DecoParamKind
  min: number
  max: number
  step: number
  defaultValue: number
  /** 是否是计数参数；计数不允许小数，表单据此把精度锁成 0。 */
  integer?: boolean
  /** 小数位数（与 `integer` 配套；不填即由 Naive 按 step 推断）。 */
  precision?: number
}

/**
 * 声明一个参数元数据，并把 `key` 的类型钉成字面量（票 #78）。
 *
 * 为什么需要它：直接把对象标成 `DecoNumberParamMeta` 会把 `key` 拓宽成
 * `string`，`DecoCharmConfig` 就没法从数组派生；而纯 `as const` 又会让
 * 省略了 `integer` / `precision` 的参数在联合类型上访问不到这两个字段
 * （表单模板要读 `param.precision`）。返回交叉类型两件事都满足。
 */
function defineDecoParam<Key extends string>(
  meta: Omit<DecoNumberParamMeta, 'key'> & { key: Key },
): DecoNumberParamMeta & { readonly key: Key } {
  return meta
}

/** 站标尺寸。 */
export const DECO_LOGO_SIZE_PARAM = defineDecoParam({
  key: 'logo_size',
  label: '站标尺寸',
  unit: 'px',
  hint: '挂件里那枚站标的显示尺寸：太小看不清是什么，太大会压住正文。',
  kind: 'positive',
  min: DECO_LOGO_SIZE_MIN,
  max: DECO_LOGO_SIZE_MAX,
  step: 1,
  defaultValue: DECO_LOGO_SIZE_DEFAULT,
})

/** 挂绳长度（票 #41）。 */
export const DECO_ROPE_LENGTH_PARAM = defineDecoParam({
  key: 'rope_length',
  label: '挂绳长度',
  unit: 'px',
  hint: '挂绳总长，同时是松垂阈值：比指针到站标的实际距离长，挂绳才会弯出弧线。',
  kind: 'positive',
  min: DECO_ROPE_LENGTH_MIN,
  max: DECO_ROPE_LENGTH_MAX,
  step: 1,
  defaultValue: DECO_ROPE_LENGTH_DEFAULT,
})

/** 链节点数（票 #41）。 */
export const DECO_SEGMENTS_PARAM = defineDecoParam({
  key: 'segments',
  label: '链节点数',
  unit: '节',
  hint: '挂绳由多少个节点串成：越多摆动越顺滑，也越费 CPU；必须是整数。',
  kind: 'segments',
  min: DECO_SEGMENTS_MIN,
  max: DECO_SEGMENTS_MAX,
  step: 1,
  defaultValue: DECO_SEGMENTS_DEFAULT,
  integer: true,
  precision: 0,
})

/** 重力加速度（票 #41）。 */
export const DECO_GRAVITY_PARAM = defineDecoParam({
  key: 'gravity',
  label: '重力',
  unit: 'px/s²',
  hint: '重力加速度：只影响甩动时的摆动速度与滞后感，不会让挂绳下垂（弧垂来自绳长）。',
  kind: 'positive',
  min: DECO_GRAVITY_MIN,
  max: DECO_GRAVITY_MAX,
  step: 10,
  defaultValue: DECO_GRAVITY_DEFAULT,
})

/** 每步阻尼（票 #41）。 */
export const DECO_DAMPING_PARAM = defineDecoParam({
  key: 'damping',
  label: '每步阻尼',
  unit: '无量纲',
  hint: '每个固定步长后保留的速度比例：越接近 1 越飘；上限 1 表示速度完全不衰减，可能配合重力持续振荡（大于 1 才是发散值，会被钳回 1）。',
  kind: 'positive',
  min: DECO_DAMPING_MIN,
  max: DECO_DAMPING_MAX,
  step: 0.001,
  defaultValue: DECO_DAMPING_DEFAULT,
})

/** 距离约束刚度：1 = 刚杆手感，越小越弹（拖动时先拉长、松手后回弹）。 */
export const DECO_STIFFNESS_PARAM = defineDecoParam({
  key: 'stiffness',
  label: '约束刚度',
  unit: '无量纲',
  hint: '距离约束的软硬：1 最硬（接近刚杆），越小越弹；拖动时先拉长再回弹，静止后仍会收敛。',
  kind: 'positive',
  min: DECO_STIFFNESS_MIN,
  max: DECO_STIFFNESS_MAX,
  step: 0.05,
  defaultValue: DECO_STIFFNESS_DEFAULT,
})

/** 描边起始宽度（票 #42）。 */
export const DECO_LINE_WIDTH_PARAM = defineDecoParam({
  key: 'line_width',
  label: '描边宽度',
  unit: 'px',
  hint: '挂绳起始处的线宽：收尾固定为它的 0.6 倍，越细越像发丝；保留一位小数。',
  kind: 'lineWidth',
  min: DECO_LINE_WIDTH_MIN,
  max: DECO_LINE_WIDTH_MAX,
  step: 0.1,
  defaultValue: DECO_LINE_WIDTH_DEFAULT,
  precision: 1,
})

/** 静止摆动频率（票 #42）。 */
export const DECO_IDLE_SWAY_HZ_PARAM = defineDecoParam({
  key: 'idle_sway_hz',
  label: '静止摆动频率',
  unit: 'Hz',
  hint: '指针静止时轻晃的快慢：0.8Hz 约 1.25 秒一个来回；填 0 也可以（配合幅度 0 关掉轻晃）。',
  kind: 'nonNegative',
  min: DECO_IDLE_SWAY_HZ_MIN,
  max: DECO_IDLE_SWAY_HZ_MAX,
  step: 0.1,
  defaultValue: DECO_IDLE_SWAY_HZ_DEFAULT,
})

/** 静止摆动幅度（票 #42）。 */
export const DECO_IDLE_SWAY_AMP_PARAM = defineDecoParam({
  key: 'idle_sway_amp',
  label: '静止摆动幅度',
  unit: 'px/s²',
  hint: '轻晃的加速度幅度：0 表示不轻晃（读者站收敛后停帧，不再有持续重绘）；越大晃得越明显。',
  kind: 'nonNegative',
  min: DECO_IDLE_SWAY_AMP_MIN,
  max: DECO_IDLE_SWAY_AMP_MAX,
  step: 1,
  defaultValue: DECO_IDLE_SWAY_AMP_DEFAULT,
})

/** 帧率上限（票 #74）。 */
export const DECO_FPS_CAP_PARAM = defineDecoParam({
  key: 'fps_cap',
  label: '帧率上限',
  unit: 'fps',
  hint: '挂件每秒最多画几次（整数）：越低越省电、越不干扰阅读；它只管画几次，不影响绳子手感。高于屏幕刷新率时等于不限。',
  kind: 'segments',
  min: DECO_FPS_CAP_MIN,
  max: DECO_FPS_CAP_MAX,
  step: 1,
  defaultValue: DECO_FPS_CAP_DEFAULT,
  integer: true,
  precision: 0,
})

/**
 * 指针端偏移 X（票 #75）。
 *
 * 它和其余参数不同：**下限是负数**，所以 `NInputNumber` 的 `min` 就是 -24。
 * 语义是"指针端节点 = 指针热点 + 偏移"：默认的 3 让指针端从光标右下悬挂。
 */
export const DECO_POINTER_OFFSET_X_PARAM = defineDecoParam({
  key: 'pointer_offset_x',
  label: '指针端偏移 X',
  unit: 'px',
  hint: '指针端相对光标热点的水平偏移（可填负数往左）：默认 3px，让指针端从右下悬挂而不是压在指尖正中。',
  kind: 'signed',
  min: DECO_POINTER_OFFSET_X_MIN,
  max: DECO_POINTER_OFFSET_X_MAX,
  step: 1,
  defaultValue: DECO_POINTER_OFFSET_X_DEFAULT,
  integer: true,
  precision: 0,
})

/** 指针端偏移 Y（票 #75）：负值往上偏；两轴到 ±24 时指针端与光标明显脱节。 */
export const DECO_POINTER_OFFSET_Y_PARAM = defineDecoParam({
  key: 'pointer_offset_y',
  label: '指针端偏移 Y',
  unit: 'px',
  hint: '指针端相对光标热点的垂直偏移（可填负数往上）：默认 3px；它同样只加在指针端节点上。',
  kind: 'signed',
  min: DECO_POINTER_OFFSET_Y_MIN,
  max: DECO_POINTER_OFFSET_Y_MAX,
  step: 1,
  defaultValue: DECO_POINTER_OFFSET_Y_DEFAULT,
  integer: true,
  precision: 0,
})

/**
 * 装饰段的数值参数清单（表单按这个顺序渲染十二行输入）。
 *
 * 总开关不是数值参数，单独一行。
 *
 * 每个参数对象的 `key` 由 `defineDecoParam` 钉成字面量，这里再用 `as const`
 * 把清单固定成只读元组：`DecoNumberParamKey` / `DecoCharmConfig` / 默认值与
 * 归一化全部由这张表派生，数组顺序不变（后台表单按它渲染）。加一个参数只改
 * 「常量 + 本表一行」。
 */
export const DECO_NUMBER_PARAMS = [
  DECO_LOGO_SIZE_PARAM,
  DECO_ROPE_LENGTH_PARAM,
  DECO_SEGMENTS_PARAM,
  DECO_LINE_WIDTH_PARAM,
  DECO_GRAVITY_PARAM,
  DECO_DAMPING_PARAM,
  DECO_STIFFNESS_PARAM,
  DECO_IDLE_SWAY_HZ_PARAM,
  DECO_IDLE_SWAY_AMP_PARAM,
  DECO_FPS_CAP_PARAM,
  DECO_POINTER_OFFSET_X_PARAM,
  DECO_POINTER_OFFSET_Y_PARAM,
] as const

/** 数值参数的键（由元数据数组派生，不再另抄一份清单）。 */
export type DecoNumberParamKey = (typeof DECO_NUMBER_PARAMS)[number]['key']

/** 数值参数共用的钳位规则：越界钳到边界、非有限与零值回落默认。 */
function clampDecoPositiveRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value <= 0) return fallback
  return Math.min(max, Math.max(min, value))
}

/** 「0 有合法含义」的数值参数共用钳位规则：0 原样保留，负数与非法回落默认。 */
function clampDecoNonNegativeRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 0) return fallback
  return Math.min(max, Math.max(min, value))
}

/**
 * 「0 与负数都是普通取值」的钳位规则 —— 第四条规则（票 #75）。
 *
 * 只夹到 [min, max]；NaN / ±Inf 回落默认。不能用另外两条顶替：
 * `clampDecoPositiveRange` 会把 <= 0 回落默认（负偏移被吃掉），
 * `clampDecoNonNegativeRange` 会把负值夹回 min（等于把"往左上偏 6px"改成
 * "偏到最大"）。
 */
function clampDecoSignedRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, value))
}

/**
 * 取整成整数，半值远离 0（-2.5 → -3）。
 *
 * 与后端 Go 的 `math.Round` 同口径：直接用 `Math.round` 会把负半值往 +∞ 取整
 * （-2.5 → -2），有符号参数上这就是跨端漂移。
 */
function roundHalfAwayFromZero(value: number): number {
  return Math.sign(value) * Math.round(Math.abs(value))
}

/**
 * 指针端偏移单轴的钳位：先夹到 `[min, max]`、再半值远离 0 取整，非有限值回落 `fallback`。
 *
 * 两轴只差常量，合用一个参数化函数（`(value, min, max, fallback)`），形状与后端
 * `normalizePointerOffset` 对齐；对外导出的 `clampDecoPointerOffsetX / Y` 只绑常量，
 * 签名不变。顺序不能反：24.6 要先夹到 24 再取整，反过来会得到 25。
 */
function clampPointerOffset(value: number, min: number, max: number, fallback: number): number {
  return roundHalfAwayFromZero(clampDecoSignedRange(value, min, max, fallback))
}

/**
 * 计数类参数（`segments` / `fps_cap`）的钳位：先夹到边界、再四舍五入成整数。
 *
 * 顺序不能反：160.6 必须得到 160，而不是 161。`segments` 早先写的是"先四舍五入
 * 再夹边界"，两者在整数边界（4–30、30–160）上结果等价 —— 共享夹具钉着边界与
 * 半值用例。
 */
function clampDecoInteger(value: number, min: number, max: number, fallback: number): number {
  return Math.round(clampDecoPositiveRange(value, min, max, fallback))
}

/**
 * 描边起始宽度的钳位：正数规则之后再保留一位小数。
 *
 * 保留一位小数是为了让输入框回填的值和提交出去的值一致（2.5400000002
 * 这种长浮点会让管理员以为是自己填错了）。
 */
function clampDecoOneDecimal(value: number, min: number, max: number, fallback: number): number {
  const clamped = clampDecoPositiveRange(value, min, max, fallback)
  // 与后端 `math.Round(v*10)/10` 同一条规则：toFixed 在半值上的舍入与
  // math.Round 不同（2.55 → "2.5" vs 2.6），跨端共享夹具会抓住这种漂移。
  return Math.round(clamped * 10) / 10
}

/**
 * 按元数据里的 `kind` 选择钳位规则 —— **归一化唯一入口**（票 #78）。
 *
 * `normalizeDecoConfig` 遍历 `DECO_NUMBER_PARAMS` 调它；下面导出的
 * `clampDecoXxx` 只是把各自那一行元数据绑上去（签名保持单参数）。加参数时
 * 这里不用动：在元数据里加一行、选一个 `kind` 就够了。
 */
export function clampDecoParam(value: number, param: DecoNumberParamMeta): number {
  switch (param.kind) {
    case 'positive':
      return clampDecoPositiveRange(value, param.min, param.max, param.defaultValue)
    case 'nonNegative':
      return clampDecoNonNegativeRange(value, param.min, param.max, param.defaultValue)
    case 'signed':
      return clampPointerOffset(value, param.min, param.max, param.defaultValue)
    case 'segments':
      return clampDecoInteger(value, param.min, param.max, param.defaultValue)
    case 'lineWidth':
      return clampDecoOneDecimal(value, param.min, param.max, param.defaultValue)
  }
}

/** 站标尺寸归一化：越界钳到边界、非正值与非法值回落默认值（0 是"没填"，不钳到 24）。 */
export function clampDecoLogoSize(value: number): number {
  return clampDecoParam(value, DECO_LOGO_SIZE_PARAM)
}

/** 挂绳长度归一化：越界钳到 40–240，零与非法值回落 120。 */
export function clampDecoRopeLength(value: number): number {
  return clampDecoParam(value, DECO_ROPE_LENGTH_PARAM)
}

/** 重力归一化：越界钳到 400–4000，零与非法值回落 1600。 */
export function clampDecoGravity(value: number): number {
  return clampDecoParam(value, DECO_GRAVITY_PARAM)
}

/** 阻尼归一化：越界钳到 0.9–1，零与非法值回落 0.99（1 合法，大于 1 夹回 1）。 */
export function clampDecoDamping(value: number): number {
  return clampDecoParam(value, DECO_DAMPING_PARAM)
}

/** 刚度归一化：越界钳到 0.2–1，零与非法值回落 0.5（0 是「没填」，不是最软）。 */
export function clampDecoStiffness(value: number): number {
  return clampDecoParam(value, DECO_STIFFNESS_PARAM)
}

/** 描边起始宽度归一化：越界钳到 1–6，零与非法值回落 2.5，并保留一位小数。 */
export function clampDecoLineWidth(value: number): number {
  return clampDecoParam(value, DECO_LINE_WIDTH_PARAM)
}

/** 静止摆动频率归一化：0 原样保留（= 关掉轻晃），越界钳到 1.5，负数/NaN 回落 0.8。 */
export function clampDecoIdleSwayHz(value: number): number {
  return clampDecoParam(value, DECO_IDLE_SWAY_HZ_PARAM)
}

/** 静止摆动幅度归一化：0 原样保留（= 停帧），越界钳到 40，负数/NaN 回落 20。 */
export function clampDecoIdleSwayAmp(value: number): number {
  return clampDecoParam(value, DECO_IDLE_SWAY_AMP_PARAM)
}

/** 帧率上限归一化：越界钳到 30–160，零与非法值回落 60，最后取整成整数。 */
export function clampDecoFpsCap(value: number): number {
  return clampDecoParam(value, DECO_FPS_CAP_PARAM)
}

/** 指针端水平偏移归一化：夹到 -24–24 后取整；0 与负值照常保留，非法值回落 3。 */
export function clampDecoPointerOffsetX(value: number): number {
  return clampDecoParam(value, DECO_POINTER_OFFSET_X_PARAM)
}

/** 指针端垂直偏移归一化：与水平分量完全对称（夹到 -24–24、取整、非法值回落 3）。 */
export function clampDecoPointerOffsetY(value: number): number {
  return clampDecoParam(value, DECO_POINTER_OFFSET_Y_PARAM)
}

/** 节点数归一化：越界钳到 4–30 再四舍五入成整数，零与非法值回落 10。 */
export function clampDecoSegments(value: number): number {
  return clampDecoParam(value, DECO_SEGMENTS_PARAM)
}

/**
 * 数字输入框回填用的短格式：整数不留小数点，声明了精度就按精度补零，
 * 其余最多保留三位（阻尼这类参数不应回填出 0.9900000001）。
 */
function formatDecoParamValue(param: DecoNumberParamMeta, value: number): string {
  if (param.integer) return String(Math.round(value))
  if (param.precision !== undefined) return value.toFixed(param.precision)
  return String(Number(value.toFixed(3)))
}

/** 一次数字输入的处理结果：夹回/回落后的值与给管理员看的提示（合法输入无提示）。 */
export interface DecoParamInputResult {
  value: number
  notice: string | null
}

/**
 * 处理数字输入框的原始值：**数字输入的钳位唯一入口**。
 *
 * 滑块本身只吐元数据区间内的值，所以它直接走 `clampDecoParam`；数字框可以被
 * 粘贴、被 devtools 改、被旧版本页面填上越界值，因此要在这里：
 *   1. 空值 / NaN 一律回落默认值，并说明「已恢复默认」；
 *   2. 按参数的 `kind` 选择与后端同一条钳位规则；
 *   3. 只要结果与输入不同就返回一句可见提示 —— 管理员不该在保存后才发现数字被改过。
 *
 * 提示文本由参数元数据拼出来（标签、区间、单位），所以新增参数不需要再来改文案。
 */
export function interpretDecoParamInput(
  raw: unknown,
  param: DecoNumberParamMeta,
): DecoParamInputResult {
  const input = raw === null || raw === undefined ? Number.NaN : Number(raw)
  const unitSuffix = param.unit ? ` ${param.unit}` : ''

  if (!Number.isFinite(input)) {
    return {
      value: param.defaultValue,
      notice: `${param.label}不是有效数字，已恢复默认 ${formatDecoParamValue(param, param.defaultValue)}${unitSuffix}`,
    }
  }

  const value = clampDecoParam(input, param)
  if (value === input) return { value, notice: null }

  // 0 / 负数在「正数规则」里是"没填或手滑"，它们回落的是默认值而不是下限；
  // 这里必须先把这类输入与"单纯越界"分开，否则提示会写成"夹回下限"。
  const invalidForKind =
    param.kind === 'positive' ? input <= 0 : param.kind === 'nonNegative' ? input < 0 : false
  if (invalidForKind) {
    return {
      value,
      notice: `${param.label}的 ${input} 不是有效值，已恢复默认 ${formatDecoParamValue(param, value)}${unitSuffix}`,
    }
  }

  if (input < param.min || input > param.max) {
    const range = `${formatDecoParamValue(param, param.min)}–${formatDecoParamValue(param, param.max)}${unitSuffix}`
    return {
      value,
      notice: `${param.label}已夹回 ${range}：${input} → ${formatDecoParamValue(param, value)}`,
    }
  }

  // 区间内但被取整/精度规则改过（例如 10.7 节 → 11 节、2.54px → 2.5px）。
  return {
    value,
    notice: `${param.label}已${param.integer ? '取整' : '按精度调整'}为 ${formatDecoParamValue(param, value)}${unitSuffix}（输入 ${input}）`,
  }
}

/**
 * 装饰段的可提交形态：总开关 + **从元数据派生的**十二个数值键（票 #78）。
 *
 * 数值键不再手写：`[K in DecoNumberParamKey]` 的键集合就是 `DECO_NUMBER_PARAMS`
 * 的 `key` 集合，加参数时类型自动跟上。
 */
export type DecoCharmConfig = { enable: boolean } & {
  [K in DecoNumberParamKey]: number
}

/** 装饰段（只含挂件子块）。 */
export interface DecoRuntimeConfigShape {
  charm: DecoCharmConfig
}

/**
 * 用元数据生成一份"每个数值键都有值"的对象。
 *
 * `pick` 决定每个键取什么值：默认值（`defaultDecoConfig`）或归一化结果
 * （`normalizeDecoConfig`）。键集合由元数据保证；`{}` 的断言只补静态类型
 * （赋值逐个键进行，键集合是否齐全由 `decoParamsConsistency.test.ts` 盯住）。
 */
function buildDecoNumericParams(
  pick: (param: (typeof DECO_NUMBER_PARAMS)[number]) => number,
): {
  [K in DecoNumberParamKey]: number
} {
  const params = {} as { [K in DecoNumberParamKey]: number }
  for (const param of DECO_NUMBER_PARAMS) {
    params[param.key] = pick(param)
  }
  return params
}

/**
 * 后台表单的"恢复默认"目标值：总开关 + 十二个数值参数，全部来自元数据里的
 * `defaultValue`（它又与 `DECO_*_DEFAULT` 常量相等，由守卫测试钉住）。
 *
 * 为什么遍历元数据而不是手写十三个字面量：恢复默认必须与后端
 * `site_default_settings.yaml` 的十三项逐项一致；默认值只留一份，后台加参数时
 * 漏改"恢复默认"在结构上就不可能发生。
 *
 * 注意它只构造表单值，**不写服务器** —— 恢复默认之后仍要管理员按保存。
 */
export function defaultDecoConfig(): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: true,
      ...buildDecoNumericParams((param) => param.defaultValue),
    },
  }
}

/** 装饰段里除总开关以外的数值键（十二个参数，由元数据派生）。 */
export type DecoCharmNumericKey = DecoNumberParamKey

/**
 * 读一个可能缺失 / 为 `null` 的数值字段：两者都折算成 NaN，交给钳位函数
 * 回落各自的默认值（票 #77）。
 *
 * 为什么不能直接 `Number(value)`：`Number(null) === 0`，而 0 在轻晃两项与
 * 两轴指针端偏移上是**合法取值** —— "这个键没有值"会被读成"管理员明确填了 0"，
 * 归一化后的表单值被当成真值提交回后端。其余值仍走 `Number(value)`：
 * 字符串数字（旧库 / 手改 JSON）保持既有的宽容度。
 */
function decoConfigNumber(value: unknown): number {
  return value === null || value === undefined ? Number.NaN : Number(value)
}

/**
 * 读回来的装饰配置的**输入**形状（票 #75 顺手校正、票 #77 放宽数值）。
 *
 * `charm` 内部也按"每个键都可能缺 / 为 null"处理，而不是要求整块
 * `DecoCharmConfig`：旧库、旧后台版本、手改的 JSON 都会只带一部分键，
 * 甚至把键写成 null（运营"清空"字段），而 `normalizeDecoConfig` 本来就是
 * 逐字段兜底的。原来的 `Partial<DecoRuntimeConfigShape>` 只允许顶层缺
 * `charm`，把"子块缺键"这件已经支持的事写成了类型错误。
 *
 * 数值键之所以是 `number | null` 而不是 `number`：`null` 与缺键同义
 * （这个键没有值），绝不等于显式 0 —— 0 在轻晃与偏移上是合法语义。
 */
export interface DecoRuntimeConfigInput {
  charm?: (Partial<Record<DecoCharmNumericKey, number | null>> & { enable?: boolean | null }) | null
}

/**
 * 把读回来的装饰配置收敛成可提交的形态。
 *
 * 三件事：缺 `charm` 子块时补默认值（开关默认开，与后端默认一致）、
 * 数值逐项走钳位（遍历元数据，按每个参数的 `kind` 选规则）、丢掉未知字段
 * （提交体里不该夹带读者站不认识的键）。
 *
 * **这是后台数值参数唯一的兜底点**：`null` / `undefined` 在这里折成 NaN，
 * 再由钳位规则回落默认值；显式 0 是数字，原样通过。
 */
export function normalizeDecoConfig(
  config: DecoRuntimeConfigInput | null | undefined,
): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: Boolean(config?.charm?.enable ?? true),
      ...buildDecoNumericParams((param) =>
        clampDecoParam(decoConfigNumber(config?.charm?.[param.key]), param),
      ),
    },
  }
}
