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
 * 本票只在单测里显式钉住这三个数，改一端会先在这里红一次。
 */

/** 站标尺寸下限（px）。 */
export const DECO_LOGO_SIZE_MIN = 24

/** 站标尺寸上限（px）。 */
export const DECO_LOGO_SIZE_MAX = 96

/** 站标尺寸默认值（px）。 */
export const DECO_LOGO_SIZE_DEFAULT = 48

/** 一个数值参数的元数据；表单与测试都只认这个形状。 */
export interface DecoNumberParamMeta {
  /** 提交给后端的键名（snake_case，与 Go 的 json tag 一致）。 */
  key: 'logo_size'
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

/**
 * 本票的数值参数清单。
 *
 * 只有一项：挂绳长度、节点数、重力、阻尼等九个参数属于票 #41，
 * 届时它们各加一行即可，表单与测试都不用改结构。
 */
export const DECO_NUMBER_PARAMS: readonly DecoNumberParamMeta[] = [DECO_LOGO_SIZE_PARAM]

/**
 * 站标尺寸归一化：越界钳到边界、非正值与非法值回落默认值。
 *
 * 0 是"字段没填"的零值，不是"我要最小"的意图 —— 所以它回落默认而不是钳到 24。
 */
export function clampDecoLogoSize(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return DECO_LOGO_SIZE_DEFAULT
  return Math.min(DECO_LOGO_SIZE_MAX, Math.max(DECO_LOGO_SIZE_MIN, value))
}

/** 装饰段的可提交形态。 */
export interface DecoCharmConfig {
  enable: boolean
  logo_size: number
}

/** 装饰段（只含挂件子块）。 */
export interface DecoRuntimeConfigShape {
  charm: DecoCharmConfig
}

/**
 * 把读回来的装饰配置收敛成可提交的形态。
 *
 * 三件事：缺 `charm` 子块时补默认值（开关默认开，与后端默认一致）、
 * 数值走钳位、丢掉未知字段（提交体里不该夹带读者站不认识的键）。
 */
export function normalizeDecoConfig(
  config: Partial<DecoRuntimeConfigShape> | null | undefined,
): DecoRuntimeConfigShape {
  return {
    charm: {
      enable: Boolean(config?.charm?.enable ?? true),
      logo_size: clampDecoLogoSize(Number(config?.charm?.logo_size)),
    },
  }
}
