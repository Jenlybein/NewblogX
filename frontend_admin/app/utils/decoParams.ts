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

/** 一个数值参数的元数据；表单与测试都只认这个形状。 */
export interface DecoNumberParamMeta {
  /** 提交给后端的键名（snake_case，与 Go 的 json tag 一致）。 */
  key:
    | 'logo_size'
    | 'rope_length'
    | 'segments'
    | 'line_width'
    | 'gravity'
    | 'damping'
    | 'idle_sway_hz'
    | 'idle_sway_amp'
    | 'fps_cap'
    | 'pointer_offset_x'
    | 'pointer_offset_y'
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
  hint: '挂绳总长，同时是松垂阈值：比指针到站标的实际距离长，挂绳才会弯出弧线。',
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
  hint: '挂绳由多少个节点串成：越多摆动越顺滑，也越费 CPU；必须是整数。',
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
  hint: '重力加速度：只影响甩动时的摆动速度与滞后感，不会让挂绳下垂（弧垂来自绳长）。',
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

/** 描边起始宽度（票 #42）。 */
export const DECO_LINE_WIDTH_PARAM: DecoNumberParamMeta = {
  key: 'line_width',
  label: '描边宽度',
  unit: 'px',
  hint: '挂绳起始处的线宽：收尾固定为它的 0.6 倍，越细越像发丝；保留一位小数。',
  min: DECO_LINE_WIDTH_MIN,
  max: DECO_LINE_WIDTH_MAX,
  step: 0.1,
  defaultValue: DECO_LINE_WIDTH_DEFAULT,
  precision: 1,
}

/** 静止摆动频率（票 #42）。 */
export const DECO_IDLE_SWAY_HZ_PARAM: DecoNumberParamMeta = {
  key: 'idle_sway_hz',
  label: '静止摆动频率',
  unit: 'Hz',
  hint: '指针静止时轻晃的快慢：0.8Hz 约 1.25 秒一个来回；填 0 也可以（配合幅度 0 关掉轻晃）。',
  min: DECO_IDLE_SWAY_HZ_MIN,
  max: DECO_IDLE_SWAY_HZ_MAX,
  step: 0.1,
  defaultValue: DECO_IDLE_SWAY_HZ_DEFAULT,
}

/** 静止摆动幅度（票 #42）。 */
export const DECO_IDLE_SWAY_AMP_PARAM: DecoNumberParamMeta = {
  key: 'idle_sway_amp',
  label: '静止摆动幅度',
  unit: 'px/s²',
  hint: '轻晃的加速度幅度：0 表示不轻晃（读者站收敛后停帧，不再有持续重绘）；越大晃得越明显。',
  min: DECO_IDLE_SWAY_AMP_MIN,
  max: DECO_IDLE_SWAY_AMP_MAX,
  step: 1,
  defaultValue: DECO_IDLE_SWAY_AMP_DEFAULT,
}

/** 帧率上限（票 #74）。 */
export const DECO_FPS_CAP_PARAM: DecoNumberParamMeta = {
  key: 'fps_cap',
  label: '帧率上限',
  unit: 'fps',
  hint: '挂件每秒最多画几次（整数）：越低越省电、越不干扰阅读；它只管画几次，不影响绳子手感。高于屏幕刷新率时等于不限。',
  min: DECO_FPS_CAP_MIN,
  max: DECO_FPS_CAP_MAX,
  step: 1,
  defaultValue: DECO_FPS_CAP_DEFAULT,
  integer: true,
  precision: 0,
}

/**
 * 指针端偏移 X（票 #75）。
 *
 * 它和其余参数不同：**下限是负数**，所以 `NInputNumber` 的 `min` 就是 -24。
 * 语义是"指针端节点 = 指针热点 + 偏移"：默认的 3 让指针端从光标右下悬挂。
 */
export const DECO_POINTER_OFFSET_X_PARAM: DecoNumberParamMeta = {
  key: 'pointer_offset_x',
  label: '指针端偏移 X',
  unit: 'px',
  hint: '指针端相对光标热点的水平偏移（可填负数往左）：默认 3px，让指针端从右下悬挂而不是压在指尖正中。',
  min: DECO_POINTER_OFFSET_X_MIN,
  max: DECO_POINTER_OFFSET_X_MAX,
  step: 1,
  defaultValue: DECO_POINTER_OFFSET_X_DEFAULT,
  integer: true,
  precision: 0,
}

/** 指针端偏移 Y（票 #75）：负值往上偏；两轴到 ±24 时指针端与光标明显脱节。 */
export const DECO_POINTER_OFFSET_Y_PARAM: DecoNumberParamMeta = {
  key: 'pointer_offset_y',
  label: '指针端偏移 Y',
  unit: 'px',
  hint: '指针端相对光标热点的垂直偏移（可填负数往上）：默认 3px；它同样只加在指针端节点上。',
  min: DECO_POINTER_OFFSET_Y_MIN,
  max: DECO_POINTER_OFFSET_Y_MAX,
  step: 1,
  defaultValue: DECO_POINTER_OFFSET_Y_DEFAULT,
  integer: true,
  precision: 0,
}

/**
 * 装饰段的数值参数清单（表单按这个顺序渲染十一行输入）。
 *
 * 总开关不是数值参数，单独一行。
 */
export const DECO_NUMBER_PARAMS: readonly DecoNumberParamMeta[] = [
  DECO_LOGO_SIZE_PARAM,
  DECO_ROPE_LENGTH_PARAM,
  DECO_SEGMENTS_PARAM,
  DECO_LINE_WIDTH_PARAM,
  DECO_GRAVITY_PARAM,
  DECO_DAMPING_PARAM,
  DECO_IDLE_SWAY_HZ_PARAM,
  DECO_IDLE_SWAY_AMP_PARAM,
  DECO_FPS_CAP_PARAM,
  DECO_POINTER_OFFSET_X_PARAM,
  DECO_POINTER_OFFSET_Y_PARAM,
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
 * 描边起始宽度归一化：越界钳到 1–6，零与非法值回落 2.5，并保留一位小数。
 *
 * 保留一位小数是为了让输入框回填的值和提交出去的值一致（2.5400000002
 * 这种长浮点会让管理员以为是自己填错了）。
 */
export function clampDecoLineWidth(value: number): number {
  const clamped = clampDecoPositiveRange(
    value,
    DECO_LINE_WIDTH_MIN,
    DECO_LINE_WIDTH_MAX,
    DECO_LINE_WIDTH_DEFAULT,
  )
  // 与后端 `math.Round(v*10)/10` 同一条规则：toFixed 在半值上的舍入与
  // math.Round 不同（2.55 → "2.5" vs 2.6），跨端共享夹具会抓住这种漂移。
  return Math.round(clamped * 10) / 10
}

/** 「0 有合法含义」的数值参数共用钳位规则：0 原样保留，负数与非法回落默认。 */
function clampDecoNonNegativeRange(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 0) return fallback
  return Math.min(max, Math.max(min, value))
}

/** 静止摆动频率归一化：0 原样保留（= 关掉轻晃），越界钳到 1.5，负数/NaN 回落 0.8。 */
export function clampDecoIdleSwayHz(value: number): number {
  return clampDecoNonNegativeRange(
    value,
    DECO_IDLE_SWAY_HZ_MIN,
    DECO_IDLE_SWAY_HZ_MAX,
    DECO_IDLE_SWAY_HZ_DEFAULT,
  )
}

/** 静止摆动幅度归一化：0 原样保留（= 停帧），越界钳到 40，负数/NaN 回落 20。 */
export function clampDecoIdleSwayAmp(value: number): number {
  return clampDecoNonNegativeRange(
    value,
    DECO_IDLE_SWAY_AMP_MIN,
    DECO_IDLE_SWAY_AMP_MAX,
    DECO_IDLE_SWAY_AMP_DEFAULT,
  )
}

/**
 * 帧率上限归一化：越界钳到 30–160，零与非法值回落 60，最后取整成整数。
 *
 * 顺序不能反（先夹边界、再取整）：160.6 必须得到 160，而不是 161。
 * 后端 `normalizeFpsCap` 与读者站 `clampCharmFpsCap` 是同一条规则，
 * 跨端共享夹具抓着这个顺序。
 */
export function clampDecoFpsCap(value: number): number {
  return Math.round(
    clampDecoPositiveRange(value, DECO_FPS_CAP_MIN, DECO_FPS_CAP_MAX, DECO_FPS_CAP_DEFAULT),
  )
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

/** 指针端水平偏移归一化：夹到 -24–24 后取整；0 与负值照常保留，非法值回落 3。 */
export function clampDecoPointerOffsetX(value: number): number {
  return clampPointerOffset(
    value,
    DECO_POINTER_OFFSET_X_MIN,
    DECO_POINTER_OFFSET_X_MAX,
    DECO_POINTER_OFFSET_X_DEFAULT,
  )
}

/** 指针端垂直偏移归一化：与水平分量完全对称（夹到 -24–24、取整、非法值回落 3）。 */
export function clampDecoPointerOffsetY(value: number): number {
  return clampPointerOffset(
    value,
    DECO_POINTER_OFFSET_Y_MIN,
    DECO_POINTER_OFFSET_Y_MAX,
    DECO_POINTER_OFFSET_Y_DEFAULT,
  )
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
  line_width: number
  gravity: number
  damping: number
  idle_sway_hz: number
  idle_sway_amp: number
  fps_cap: number
  pointer_offset_x: number
  pointer_offset_y: number
}

/** 装饰段（只含挂件子块）。 */
export interface DecoRuntimeConfigShape {
  charm: DecoCharmConfig
}

/**
 * 后台表单的"恢复默认"目标值：总开关 + 十一个数值参数，全部来自上面那组常量。
 *
 * 为什么单独成一个函数而不是在组件里写十二个字面量：恢复默认必须与后端
 * `site_default_settings.yaml` 的十二项逐项一致；把默认值只留在这里，后台
 * 加参数时漏改"恢复默认"会立刻被工具层测试抓到。
 *
 * 注意它只构造表单值，**不写服务器** —— 恢复默认之后仍要管理员按保存。
 */
export function defaultDecoConfig(): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: true,
      logo_size: DECO_LOGO_SIZE_DEFAULT,
      rope_length: DECO_ROPE_LENGTH_DEFAULT,
      segments: DECO_SEGMENTS_DEFAULT,
      line_width: DECO_LINE_WIDTH_DEFAULT,
      gravity: DECO_GRAVITY_DEFAULT,
      damping: DECO_DAMPING_DEFAULT,
      idle_sway_hz: DECO_IDLE_SWAY_HZ_DEFAULT,
      idle_sway_amp: DECO_IDLE_SWAY_AMP_DEFAULT,
      fps_cap: DECO_FPS_CAP_DEFAULT,
      pointer_offset_x: DECO_POINTER_OFFSET_X_DEFAULT,
      pointer_offset_y: DECO_POINTER_OFFSET_Y_DEFAULT,
    },
  }
}

/**
 * 读回来的装饰配置的**输入**形状（票 #75 顺手校正）。
 *
 * `charm` 内部也按"每个键都可能缺"处理，而不是要求整块 `DecoCharmConfig`：
 * 旧库、旧后台版本、手改的 JSON 都会只带一部分键，而 `normalizeDecoConfig`
 * 本来就是逐字段兜底的。原来的 `Partial<DecoRuntimeConfigShape>` 只允许顶层缺
 * `charm`，把"子块缺键"这件已经支持的事写成了类型错误。
 */
export interface DecoRuntimeConfigInput {
  charm?: Partial<DecoCharmConfig> | null
}

/**
 * 把读回来的装饰配置收敛成可提交的形态。
 *
 * 三件事：缺 `charm` 子块时补默认值（开关默认开，与后端默认一致）、
 * 数值逐项走钳位、丢掉未知字段（提交体里不该夹带读者站不认识的键）。
 */
export function normalizeDecoConfig(
  config: DecoRuntimeConfigInput | null | undefined,
): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: Boolean(config?.charm?.enable ?? true),
      logo_size: clampDecoLogoSize(Number(config?.charm?.logo_size)),
      rope_length: clampDecoRopeLength(Number(config?.charm?.rope_length)),
      segments: clampDecoSegments(Number(config?.charm?.segments)),
      line_width: clampDecoLineWidth(Number(config?.charm?.line_width)),
      gravity: clampDecoGravity(Number(config?.charm?.gravity)),
      damping: clampDecoDamping(Number(config?.charm?.damping)),
      idle_sway_hz: clampDecoIdleSwayHz(Number(config?.charm?.idle_sway_hz)),
      idle_sway_amp: clampDecoIdleSwayAmp(Number(config?.charm?.idle_sway_amp)),
      fps_cap: clampDecoFpsCap(Number(config?.charm?.fps_cap)),
      pointer_offset_x: clampDecoPointerOffsetX(Number(config?.charm?.pointer_offset_x)),
      pointer_offset_y: clampDecoPointerOffsetY(Number(config?.charm?.pointer_offset_y)),
    },
  }
}
