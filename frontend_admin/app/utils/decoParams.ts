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
export const DECO_LOGO_SIZE_MIN = 24

/** 站标尺寸上限（px）。 */
export const DECO_LOGO_SIZE_MAX = 96

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
export const DECO_SEGMENTS_MAX = 24

/** 链节点数默认值（节）。 */
export const DECO_SEGMENTS_DEFAULT = 10

/** 重力加速度下限（px/s²）。 */
export const DECO_GRAVITY_MIN = 400

/** 重力加速度上限（px/s²）。 */
export const DECO_GRAVITY_MAX = 4000

/** 重力加速度默认值（px/s²）。 */
export const DECO_GRAVITY_DEFAULT = 1600

/** 每步速度保持系数下限（无量纲）。 */
export const DECO_DAMPING_MIN = 0.94

/** 每步速度保持系数上限（无量纲，必须小于 1）。 */
export const DECO_DAMPING_MAX = 0.995

/** 每步速度保持系数默认值（无量纲）。 */
export const DECO_DAMPING_DEFAULT = 0.99

/** 一个数值参数的元数据；表单与测试都只认这个形状。 */
export interface DecoNumberParamMeta {
  /** 提交给后端的键名（snake_case，与 Go 的 json tag 一致）。 */
  key: 'logo_size' | 'rope_length' | 'segments' | 'gravity' | 'damping'
  /** 表单项标签。 */
  label: string
  /** 单位后缀，直接显示在输入框右侧。 */
  unit: string
  /** 一句话说明：这个参数影响什么。 */
  hint: string
  min: number
  max: number
  step: number
  defaultValue: number
  /** 是否是计数参数；计数不允许小数，表单据此把精度锁成 0。 */
  integer?: boolean
  /** 小数位数（与 `integer` 配套；不填即由 Naive 按 step 推断）。 */
  precision?: number
}

/** 站标尺寸。 */
export const DECO_LOGO_SIZE_PARAM: DecoNumberParamMeta = {
  key: 'logo_size',
  label: '站标尺寸',
  unit: 'px',
  hint: '挂件里那枚站标的显示尺寸：太小看不清是什么，太大会压住正文。',
  min: DECO_LOGO_SIZE_MIN,
  max: DECO_LOGO_SIZE_MAX,
  step: 1,
  defaultValue: DECO_LOGO_SIZE_DEFAULT,
}

/** 挂绳长度（票 #41）。 */
export const DECO_ROPE_LENGTH_PARAM: DecoNumberParamMeta = {
  key: 'rope_length',
  label: '挂绳长度',
  unit: 'px',
  hint: '挂绳总长，同时是松垂阈值：比指针到站标的实际距离长，绳子才会弯出弧线。',
  min: DECO_ROPE_LENGTH_MIN,
  max: DECO_ROPE_LENGTH_MAX,
  step: 1,
  defaultValue: DECO_ROPE_LENGTH_DEFAULT,
}

/** 链节点数（票 #41）。 */
export const DECO_SEGMENTS_PARAM: DecoNumberParamMeta = {
  key: 'segments',
  label: '链节点数',
  unit: '节',
  hint: '绳子由多少个节点串成：越多摆动越顺滑，也越费 CPU；必须是整数。',
  min: DECO_SEGMENTS_MIN,
  max: DECO_SEGMENTS_MAX,
  step: 1,
  defaultValue: DECO_SEGMENTS_DEFAULT,
  integer: true,
  precision: 0,
}

/** 重力加速度（票 #41）。 */
export const DECO_GRAVITY_PARAM: DecoNumberParamMeta = {
  key: 'gravity',
  label: '重力',
  unit: 'px/s²',
  hint: '重力加速度：只影响甩动时的摆动速度与滞后感，不会让绳子下垂（弧垂来自绳长）。',
  min: DECO_GRAVITY_MIN,
  max: DECO_GRAVITY_MAX,
  step: 10,
  defaultValue: DECO_GRAVITY_DEFAULT,
}

/** 每步阻尼（票 #41）。 */
export const DECO_DAMPING_PARAM: DecoNumberParamMeta = {
  key: 'damping',
  label: '每步阻尼',
  unit: '无量纲',
  hint: '每个固定步长后保留的速度比例：越接近 1 越飘；上限 0.995，再大摆动会越甩越大。',
  min: DECO_DAMPING_MIN,
  max: DECO_DAMPING_MAX,
  step: 0.001,
  defaultValue: DECO_DAMPING_DEFAULT,
}

/**
 * 装饰段的数值参数清单（表单按这个顺序渲染五行输入）。
 *
 * 总开关不是数值参数，单独一行。
 */
export const DECO_NUMBER_PARAMS: readonly DecoNumberParamMeta[] = [
  DECO_LOGO_SIZE_PARAM,
  DECO_ROPE_LENGTH_PARAM,
  DECO_SEGMENTS_PARAM,
  DECO_GRAVITY_PARAM,
  DECO_DAMPING_PARAM,
]

/** 数值参数共用的钳位规则：越界钳到边界、非有限与零值回落默认。 */
function clampDecoPositiveRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value <= 0) return fallback
  return Math.min(max, Math.max(min, value))
}

/**
 * 站标尺寸归一化：越界钳到边界、非正值与非法值回落默认值。
 *
 * 0 是"字段没填"的零值，不是"我要最小"的意图 —— 所以它回落默认而不是钳到 24。
 */
export function clampDecoLogoSize(value: number): number {
  return clampDecoPositiveRange(value, DECO_LOGO_SIZE_MIN, DECO_LOGO_SIZE_MAX, DECO_LOGO_SIZE_DEFAULT)
}

/** 挂绳长度归一化：越界钳到 40–240，零与非法值回落 120。 */
export function clampDecoRopeLength(value: number): number {
  return clampDecoPositiveRange(
    value,
    DECO_ROPE_LENGTH_MIN,
    DECO_ROPE_LENGTH_MAX,
    DECO_ROPE_LENGTH_DEFAULT,
  )
}

/** 重力归一化：越界钳到 400–4000，零与非法值回落 1600。 */
export function clampDecoGravity(value: number): number {
  return clampDecoPositiveRange(value, DECO_GRAVITY_MIN, DECO_GRAVITY_MAX, DECO_GRAVITY_DEFAULT)
}

/** 阻尼归一化：越界钳到 0.94–0.995，零与非法值回落 0.99。 */
export function clampDecoDamping(value: number): number {
  return clampDecoPositiveRange(value, DECO_DAMPING_MIN, DECO_DAMPING_MAX, DECO_DAMPING_DEFAULT)
}

/**
 * 节点数归一化：计数先四舍五入，再钳到 4–24，零与非法值回落 10。
 *
 * 与读者站同一条规则；后台先钳一次，提交出去的就是整数。
 */
export function clampDecoSegments(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return DECO_SEGMENTS_DEFAULT
  return Math.min(DECO_SEGMENTS_MAX, Math.max(DECO_SEGMENTS_MIN, Math.round(value)))
}

/** 装饰段的可提交形态。 */
export interface DecoCharmConfig {
  enable: boolean
  logo_size: number
  rope_length: number
  segments: number
  gravity: number
  damping: number
}

/** 装饰段（只含挂件子块）。 */
export interface DecoRuntimeConfigShape {
  charm: DecoCharmConfig
}

/**
 * 把读回来的装饰配置收敛成可提交的形态。
 *
 * 三件事：缺 `charm` 子块时补默认值（开关默认开，与后端默认一致）、
 * 数值逐项走钳位、丢掉未知字段（提交体里不该夹带读者站不认识的键）。
 */
export function normalizeDecoConfig(
  config: Partial<DecoRuntimeConfigShape> | null | undefined,
): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: Boolean(config?.charm?.enable ?? true),
      logo_size: clampDecoLogoSize(Number(config?.charm?.logo_size)),
      rope_length: clampDecoRopeLength(Number(config?.charm?.rope_length)),
      segments: clampDecoSegments(Number(config?.charm?.segments)),
      gravity: clampDecoGravity(Number(config?.charm?.gravity)),
      damping: clampDecoDamping(Number(config?.charm?.damping)),
    },
  }
}
