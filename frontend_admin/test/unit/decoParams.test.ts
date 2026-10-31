import { describe, expect, it } from 'vitest'
import {
  DECO_LOGO_SIZE_DEFAULT,
  DECO_LOGO_SIZE_MAX,
  DECO_LOGO_SIZE_MIN,
  DECO_LOGO_SIZE_PARAM,
  DECO_NUMBER_PARAMS,
  clampDecoLogoSize,
  normalizeDecoConfig,
} from '~/utils/decoParams'

/**
 * 为什么值得测：后台这一层是"管理员填了离谱数值"的最后一道防线。
 * 表单的 NInputNumber 只是 UI 约束（可以被粘贴、被 devtools 改掉），
 * 真正保证库里存不下发散值的是这里的钳位；而元数据（标签/单位/范围/默认值）
 * 一旦与后端漂移，就会出现"后台能填、读者站不认"。
 */
describe('装饰参数元数据', () => {
  it('站标尺寸带单位与一句作用说明（管理员不用读代码猜）', () => {
    expect(DECO_LOGO_SIZE_PARAM.key).toBe('logo_size')
    expect(DECO_LOGO_SIZE_PARAM.label).toBe('站标尺寸')
    expect(DECO_LOGO_SIZE_PARAM.unit).toBe('px')
    expect(DECO_LOGO_SIZE_PARAM.hint.length).toBeGreaterThan(8)
  })

  it('区间与默认值：24–96，默认 48（与后端 conf_deco.go 同一组数）', () => {
    // 这三个数是跨端契约。本票不做一致性门禁（票 #43），所以在这里显式钉住：
    // 改一端就必须同时改另一端，否则这条断言会逼你想一次。
    expect(DECO_LOGO_SIZE_MIN).toBe(24)
    expect(DECO_LOGO_SIZE_MAX).toBe(96)
    expect(DECO_LOGO_SIZE_DEFAULT).toBe(48)
    expect(DECO_LOGO_SIZE_PARAM.min).toBe(DECO_LOGO_SIZE_MIN)
    expect(DECO_LOGO_SIZE_PARAM.max).toBe(DECO_LOGO_SIZE_MAX)
    expect(DECO_LOGO_SIZE_PARAM.defaultValue).toBe(DECO_LOGO_SIZE_DEFAULT)
    expect(DECO_LOGO_SIZE_PARAM.defaultValue).toBeGreaterThanOrEqual(DECO_LOGO_SIZE_MIN)
    expect(DECO_LOGO_SIZE_PARAM.defaultValue).toBeLessThanOrEqual(DECO_LOGO_SIZE_MAX)
    expect(DECO_LOGO_SIZE_PARAM.step).toBeGreaterThan(0)
  })

  it('参数清单本票只有站标尺寸一项（开关不是数值参数）', () => {
    expect(DECO_NUMBER_PARAMS.map((param) => param.key)).toEqual(['logo_size'])
  })
})

describe('clampDecoLogoSize', () => {
  it('越界钳到边界', () => {
    expect(clampDecoLogoSize(4000)).toBe(DECO_LOGO_SIZE_MAX)
    expect(clampDecoLogoSize(96.5)).toBe(DECO_LOGO_SIZE_MAX)
    expect(clampDecoLogoSize(10)).toBe(DECO_LOGO_SIZE_MIN)
  })

  it('区间内与边界值原样保留', () => {
    expect(clampDecoLogoSize(60)).toBe(60)
    expect(clampDecoLogoSize(24)).toBe(24)
    expect(clampDecoLogoSize(96)).toBe(96)
  })

  it('缺省（0）与非法值回落默认值', () => {
    expect(clampDecoLogoSize(0)).toBe(DECO_LOGO_SIZE_DEFAULT)
    expect(clampDecoLogoSize(-5)).toBe(DECO_LOGO_SIZE_DEFAULT)
    expect(clampDecoLogoSize(Number.NaN)).toBe(DECO_LOGO_SIZE_DEFAULT)
    expect(clampDecoLogoSize(Number.POSITIVE_INFINITY)).toBe(DECO_LOGO_SIZE_DEFAULT)
  })
})

describe('normalizeDecoConfig', () => {
  it('缺子块时补默认值（开关默认开、尺寸取默认）', () => {
    expect(normalizeDecoConfig({} as never)).toEqual({
      charm: { enable: true, logo_size: DECO_LOGO_SIZE_DEFAULT },
    })
  })

  it('保留管理员显式关掉的开关，同时钳住尺寸', () => {
    expect(normalizeDecoConfig({ charm: { enable: false, logo_size: 999 } })).toEqual({
      charm: { enable: false, logo_size: DECO_LOGO_SIZE_MAX },
    })
  })

  it('提交出去的形状只有 charm 子块（不夹带未知字段）', () => {
    const normalized = normalizeDecoConfig({
      charm: { enable: true, logo_size: 30 },
      extra: 'x',
    } as never)
    expect(Object.keys(normalized)).toEqual(['charm'])
  })
})
