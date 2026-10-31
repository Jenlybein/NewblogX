import { describe, expect, it } from 'vitest'
import {
  DECO_DAMPING_DEFAULT,
  DECO_DAMPING_MAX,
  DECO_DAMPING_MIN,
  DECO_GRAVITY_DEFAULT,
  DECO_GRAVITY_MAX,
  DECO_GRAVITY_MIN,
  DECO_LOGO_SIZE_DEFAULT,
  DECO_LOGO_SIZE_MAX,
  DECO_LOGO_SIZE_MIN,
  DECO_LOGO_SIZE_PARAM,
  DECO_NUMBER_PARAMS,
  DECO_ROPE_LENGTH_DEFAULT,
  DECO_ROPE_LENGTH_MAX,
  DECO_ROPE_LENGTH_MIN,
  DECO_SEGMENTS_DEFAULT,
  DECO_SEGMENTS_MAX,
  DECO_SEGMENTS_MIN,
  clampDecoDamping,
  clampDecoGravity,
  clampDecoLogoSize,
  clampDecoRopeLength,
  clampDecoSegments,
  normalizeDecoConfig,
} from '~/utils/decoParams'

/**
 * 为什么值得测：后台这一层是"管理员填了离谱数值"的最后一道防线。
 * 表单的 NInputNumber 只是 UI 约束（可以被粘贴、被 devtools 改掉），
 * 真正保证库里存不下发散值的是这里的钳位；而元数据（标签/单位/范围/默认值）
 * 一旦与后端漂移，就会出现"后台能填、读者站不认"。
 *
 * 票 #41 新增四个物理参数后，这里还要保证"参数清单 = 表单渲染的输入行"：
 * 少一项就是后台根本改不了，多项就是提交了读者站不认识的键。
 */

/** 元数据表里的参数越多，后台可调的东西越多；缺单位/说明就没法看。 */
const PHYSICS_PARAMS = DECO_NUMBER_PARAMS.filter((param) => param.key !== 'logo_size')

describe('装饰参数元数据', () => {
  it('站标尺寸带单位与一句作用说明（管理员不用读代码猜）', () => {
    expect(DECO_LOGO_SIZE_PARAM.key).toBe('logo_size')
    expect(DECO_LOGO_SIZE_PARAM.label).toBe('站标尺寸')
    expect(DECO_LOGO_SIZE_PARAM.unit).toBe('px')
    expect(DECO_LOGO_SIZE_PARAM.hint.length).toBeGreaterThan(8)
  })

  it('四个物理参数的标签 / 单位 / 作用说明齐全，且单位与量纲对应', () => {
    const byKey = new Map(DECO_NUMBER_PARAMS.map((param) => [param.key, param]))

    expect(byKey.get('rope_length')?.label).toBe('挂绳长度')
    expect(byKey.get('rope_length')?.unit).toBe('px')
    expect(byKey.get('segments')?.label).toBe('链节点数')
    expect(byKey.get('segments')?.unit).toBe('节')
    expect(byKey.get('gravity')?.label).toBe('重力')
    expect(byKey.get('gravity')?.unit).toBe('px/s²')
    expect(byKey.get('damping')?.label).toBe('每步阻尼')
    expect(byKey.get('damping')?.unit).toBe('无量纲')
    for (const param of PHYSICS_PARAMS) {
      // 一句说明就是"这个数影响什么"，不能只留标签。
      expect(param.hint.length).toBeGreaterThan(8)
      expect(param.max).toBeGreaterThan(param.min)
      expect(param.step).toBeGreaterThan(0)
    }
  })

  it('区间与默认值：24–96/48、40–240/120、4–24/10、400–4000/1600、0.94–0.995/0.99', () => {
    // 这些数是跨端契约。本票不做一致性门禁（票 #43），所以在这里显式钉住：
    // 改一端就必须同时改另一端，否则这条断言会逼你想一次。
    expect(DECO_LOGO_SIZE_MIN).toBe(24)
    expect(DECO_LOGO_SIZE_MAX).toBe(96)
    expect(DECO_LOGO_SIZE_DEFAULT).toBe(48)

    expect(DECO_ROPE_LENGTH_MIN).toBe(40)
    expect(DECO_ROPE_LENGTH_MAX).toBe(240)
    expect(DECO_ROPE_LENGTH_DEFAULT).toBe(120)

    expect(DECO_SEGMENTS_MIN).toBe(4)
    expect(DECO_SEGMENTS_MAX).toBe(24)
    expect(DECO_SEGMENTS_DEFAULT).toBe(10)

    expect(DECO_GRAVITY_MIN).toBe(400)
    expect(DECO_GRAVITY_MAX).toBe(4000)
    expect(DECO_GRAVITY_DEFAULT).toBe(1600)

    expect(DECO_DAMPING_MIN).toBe(0.94)
    expect(DECO_DAMPING_MAX).toBe(0.995)
    expect(DECO_DAMPING_DEFAULT).toBe(0.99)

    // 阻尼上限必须小于 1：等于或大于 1 会让摆动越甩越大。
    expect(DECO_DAMPING_MAX).toBeLessThan(1)
  })

  it('参数清单就是表单要渲染的五行（开关不是数值参数）', () => {
    expect(DECO_NUMBER_PARAMS.map((param) => param.key)).toEqual([
      'logo_size',
      'rope_length',
      'segments',
      'gravity',
      'damping',
    ])
  })

  it('每个参数的元数据区间与常量同源（表单不自己写数字）', () => {
    const expected = {
      logo_size: { min: DECO_LOGO_SIZE_MIN, max: DECO_LOGO_SIZE_MAX, defaultValue: DECO_LOGO_SIZE_DEFAULT },
      rope_length: { min: DECO_ROPE_LENGTH_MIN, max: DECO_ROPE_LENGTH_MAX, defaultValue: DECO_ROPE_LENGTH_DEFAULT },
      segments: { min: DECO_SEGMENTS_MIN, max: DECO_SEGMENTS_MAX, defaultValue: DECO_SEGMENTS_DEFAULT },
      gravity: { min: DECO_GRAVITY_MIN, max: DECO_GRAVITY_MAX, defaultValue: DECO_GRAVITY_DEFAULT },
      damping: { min: DECO_DAMPING_MIN, max: DECO_DAMPING_MAX, defaultValue: DECO_DAMPING_DEFAULT },
    } as const

    for (const param of DECO_NUMBER_PARAMS) {
      const contract = expected[param.key]
      expect(param.min).toBe(contract.min)
      expect(param.max).toBe(contract.max)
      expect(param.defaultValue).toBe(contract.defaultValue)
      expect(param.defaultValue).toBeGreaterThanOrEqual(contract.min)
      expect(param.defaultValue).toBeLessThanOrEqual(contract.max)
    }
  })

  it('节点数是计数：元数据标成整数并强制 0 位小数', () => {
    const segments = DECO_NUMBER_PARAMS.find((param) => param.key === 'segments')

    expect(segments?.integer).toBe(true)
    expect(segments?.precision).toBe(0)
    // 其余参数允许小数（阻尼必须能填 0.97）。
    expect(DECO_NUMBER_PARAMS.find((param) => param.key === 'damping')?.integer).toBeFalsy()
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

describe('clampDecoRopeLength / clampDecoGravity / clampDecoDamping', () => {
  it('挂绳长度：越界钳到 40/240，缺省与非法回落 120', () => {
    expect(clampDecoRopeLength(4000)).toBe(DECO_ROPE_LENGTH_MAX)
    expect(clampDecoRopeLength(10)).toBe(DECO_ROPE_LENGTH_MIN)
    expect(clampDecoRopeLength(200)).toBe(200)
    expect(clampDecoRopeLength(0)).toBe(DECO_ROPE_LENGTH_DEFAULT)
    expect(clampDecoRopeLength(-5)).toBe(DECO_ROPE_LENGTH_DEFAULT)
    expect(clampDecoRopeLength(Number.NaN)).toBe(DECO_ROPE_LENGTH_DEFAULT)
  })

  it('重力：越界钳到 400/4000，缺省与非法回落 1600', () => {
    expect(clampDecoGravity(100000)).toBe(DECO_GRAVITY_MAX)
    expect(clampDecoGravity(100)).toBe(DECO_GRAVITY_MIN)
    expect(clampDecoGravity(1800)).toBe(1800)
    expect(clampDecoGravity(0)).toBe(DECO_GRAVITY_DEFAULT)
    expect(clampDecoGravity(-1)).toBe(DECO_GRAVITY_DEFAULT)
    expect(clampDecoGravity(Number.NaN)).toBe(DECO_GRAVITY_DEFAULT)
  })

  it('阻尼：越界钳到 0.94/0.995，缺省与非法回落 0.99，等于 1 被钳回上限', () => {
    expect(clampDecoDamping(2)).toBe(DECO_DAMPING_MAX)
    expect(clampDecoDamping(1)).toBe(DECO_DAMPING_MAX)
    expect(clampDecoDamping(0.5)).toBe(DECO_DAMPING_MIN)
    expect(clampDecoDamping(0.97)).toBe(0.97)
    expect(clampDecoDamping(0)).toBe(DECO_DAMPING_DEFAULT)
    expect(clampDecoDamping(-0.2)).toBe(DECO_DAMPING_DEFAULT)
    expect(clampDecoDamping(Number.NaN)).toBe(DECO_DAMPING_DEFAULT)
  })
})

describe('clampDecoSegments', () => {
  it('越界钳到 4/24，并四舍五入成整数（不接受"半节"）', () => {
    expect(clampDecoSegments(100)).toBe(DECO_SEGMENTS_MAX)
    expect(clampDecoSegments(1)).toBe(DECO_SEGMENTS_MIN)
    expect(clampDecoSegments(10.7)).toBe(11)
    expect(clampDecoSegments(4)).toBe(4)
    expect(clampDecoSegments(24)).toBe(24)
  })

  it('缺省与非法值回落默认 10', () => {
    expect(clampDecoSegments(0)).toBe(DECO_SEGMENTS_DEFAULT)
    expect(clampDecoSegments(-3)).toBe(DECO_SEGMENTS_DEFAULT)
    expect(clampDecoSegments(Number.NaN)).toBe(DECO_SEGMENTS_DEFAULT)
    expect(clampDecoSegments(Number.POSITIVE_INFINITY)).toBe(DECO_SEGMENTS_DEFAULT)
  })
})

describe('normalizeDecoConfig', () => {
  it('缺子块时补默认值（开关默认开、五个数值都取默认）', () => {
    expect(normalizeDecoConfig({} as never)).toEqual({
      charm: {
        enable: true,
        logo_size: DECO_LOGO_SIZE_DEFAULT,
        rope_length: DECO_ROPE_LENGTH_DEFAULT,
        segments: DECO_SEGMENTS_DEFAULT,
        gravity: DECO_GRAVITY_DEFAULT,
        damping: DECO_DAMPING_DEFAULT,
      },
    })
  })

  it('保留管理员显式关掉的开关，同时钳住全部五个数值', () => {
    expect(
      normalizeDecoConfig({
        charm: {
          enable: false,
          logo_size: 999,
          rope_length: 9999,
          segments: 100,
          gravity: 99999,
          damping: 2,
        },
      }),
    ).toEqual({
      charm: {
        enable: false,
        logo_size: DECO_LOGO_SIZE_MAX,
        rope_length: DECO_ROPE_LENGTH_MAX,
        segments: DECO_SEGMENTS_MAX,
        gravity: DECO_GRAVITY_MAX,
        damping: DECO_DAMPING_MAX,
      },
    })
  })

  it('部分字段缺失只影响它自己（其余保持传入值）', () => {
    expect(normalizeDecoConfig({ charm: { enable: true, rope_length: 80 } }).charm).toEqual({
      enable: true,
      logo_size: DECO_LOGO_SIZE_DEFAULT,
      rope_length: 80,
      segments: DECO_SEGMENTS_DEFAULT,
      gravity: DECO_GRAVITY_DEFAULT,
      damping: DECO_DAMPING_DEFAULT,
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
