import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'
import {
  DECO_DAMPING_DEFAULT,
  DECO_DAMPING_MAX,
  DECO_DAMPING_MIN,
  DECO_FPS_CAP_DEFAULT,
  DECO_FPS_CAP_MAX,
  DECO_FPS_CAP_MIN,
  DECO_GRAVITY_DEFAULT,
  DECO_GRAVITY_MAX,
  DECO_GRAVITY_MIN,
  DECO_IDLE_SWAY_AMP_DEFAULT,
  DECO_IDLE_SWAY_AMP_MAX,
  DECO_IDLE_SWAY_AMP_MIN,
  DECO_IDLE_SWAY_HZ_DEFAULT,
  DECO_IDLE_SWAY_HZ_MAX,
  DECO_IDLE_SWAY_HZ_MIN,
  DECO_LINE_WIDTH_DEFAULT,
  DECO_LINE_WIDTH_MAX,
  DECO_LINE_WIDTH_MIN,
  DECO_LOGO_SIZE_DEFAULT,
  DECO_LOGO_SIZE_MAX,
  DECO_LOGO_SIZE_MIN,
  DECO_LOGO_SIZE_PARAM,
  DECO_NUMBER_PARAMS,
  DECO_POINTER_OFFSET_X_DEFAULT,
  DECO_POINTER_OFFSET_X_MAX,
  DECO_POINTER_OFFSET_X_MIN,
  DECO_POINTER_OFFSET_Y_DEFAULT,
  DECO_POINTER_OFFSET_Y_MAX,
  DECO_POINTER_OFFSET_Y_MIN,
  DECO_ROPE_LENGTH_DEFAULT,
  DECO_ROPE_LENGTH_MAX,
  DECO_ROPE_LENGTH_MIN,
  DECO_SEGMENTS_DEFAULT,
  DECO_SEGMENTS_MAX,
  DECO_SEGMENTS_MIN,
  DECO_STIFFNESS_DEFAULT,
  DECO_STIFFNESS_MAX,
  DECO_STIFFNESS_MIN,
  clampDecoDamping,
  clampDecoFpsCap,
  clampDecoGravity,
  clampDecoIdleSwayAmp,
  clampDecoIdleSwayHz,
  clampDecoLineWidth,
  clampDecoLogoSize,
  clampDecoPointerOffsetX,
  clampDecoPointerOffsetY,
  clampDecoRopeLength,
  clampDecoSegments,
  clampDecoStiffness,
  defaultDecoConfig,
  normalizeDecoConfig,
} from '~/utils/decoParams'

/**
 * 为什么值得测：后台这一层是"管理员填了离谱数值"的最后一道防线。
 * 表单的 NInputNumber 只是 UI 约束（可以被粘贴、被 devtools 改掉），
 * 真正保证库里存不下发散值的是这里的钳位；而元数据（标签/单位/范围/默认值）
 * 一旦与后端漂移，就会出现"后台能填、读者站不认"。
 *
 * 票 #41 新增四个物理参数、票 #42 再新增三个（描边宽度 + 静止轻晃的两个）、
 * 票 #74 新增帧率上限、票 #75 新增指针端偏移 X / Y。这里还保证三件事：参数清单 =
 * 表单渲染的输入行；每个参数都有单位与一句作用说明；「恢复默认」的十二个值与
 * 规范默认逐项一致（且只是表单值）。
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

  it('十二个数值参数的标签 / 单位 / 作用说明齐全，且单位与量纲对应', () => {
    const byKey = new Map(DECO_NUMBER_PARAMS.map((param) => [param.key, param]))

    expect(byKey.get('rope_length')?.label).toBe('挂绳长度')
    expect(byKey.get('rope_length')?.unit).toBe('px')
    expect(byKey.get('segments')?.label).toBe('链节点数')
    expect(byKey.get('segments')?.unit).toBe('节')
    expect(byKey.get('line_width')?.label).toBe('描边宽度')
    expect(byKey.get('line_width')?.unit).toBe('px')
    expect(byKey.get('gravity')?.label).toBe('重力')
    expect(byKey.get('gravity')?.unit).toBe('px/s²')
    expect(byKey.get('damping')?.label).toBe('每步阻尼')
    expect(byKey.get('damping')?.unit).toBe('无量纲')
    // 刚度也是无量纲比例：1 最硬，越小越弹。
    expect(byKey.get('stiffness')?.label).toBe('约束刚度')
    expect(byKey.get('stiffness')?.unit).toBe('无量纲')
    expect(byKey.get('idle_sway_hz')?.label).toBe('静止摆动频率')
    expect(byKey.get('idle_sway_hz')?.unit).toBe('Hz')
    expect(byKey.get('idle_sway_amp')?.label).toBe('静止摆动幅度')
    expect(byKey.get('idle_sway_amp')?.unit).toBe('px/s²')
    // 票 #74：帧率上限的单位是 fps，不是 px —— 它是"每秒画几次"。
    expect(byKey.get('fps_cap')?.label).toBe('帧率上限')
    expect(byKey.get('fps_cap')?.unit).toBe('fps')
    // 票 #75：指针端偏移是长度，单位是 px；两轴各有自己的一行。
    expect(byKey.get('pointer_offset_x')?.label).toBe('指针端偏移 X')
    expect(byKey.get('pointer_offset_x')?.unit).toBe('px')
    expect(byKey.get('pointer_offset_y')?.label).toBe('指针端偏移 Y')
    expect(byKey.get('pointer_offset_y')?.unit).toBe('px')
    for (const param of PHYSICS_PARAMS) {
      // 一句说明就是"这个数影响什么"，不能只留标签。
      expect(param.hint.length).toBeGreaterThan(8)
      expect(param.max).toBeGreaterThan(param.min)
      expect(param.step).toBeGreaterThan(0)
    }
  })

  it('区间与默认值：24–96/48、40–240/120、4–24/10、1–6/2.5、400–4000/1600、0.94–0.995/0.99、0.2–1/0.5、0–1.5/0.8、0–40/20、30–160/60、-24–24/3', () => {
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

    expect(DECO_LINE_WIDTH_MIN).toBe(1)
    expect(DECO_LINE_WIDTH_MAX).toBe(6)
    expect(DECO_LINE_WIDTH_DEFAULT).toBe(2.5)

    expect(DECO_GRAVITY_MIN).toBe(400)
    expect(DECO_GRAVITY_MAX).toBe(4000)
    expect(DECO_GRAVITY_DEFAULT).toBe(1600)

    expect(DECO_DAMPING_MIN).toBe(0.94)
    expect(DECO_DAMPING_MAX).toBe(0.995)
    expect(DECO_DAMPING_DEFAULT).toBe(0.99)
    // 阻尼上限必须小于 1：等于或大于 1 会让摆动越甩越大。
    expect(DECO_DAMPING_MAX).toBeLessThan(1)

    // 刚度上限恰好是 1（全量投影），下限 0.2（更软就久久回不到位）。
    expect(DECO_STIFFNESS_MIN).toBe(0.2)
    expect(DECO_STIFFNESS_MAX).toBe(1)
    expect(DECO_STIFFNESS_DEFAULT).toBe(0.5)

    expect(DECO_IDLE_SWAY_HZ_MIN).toBe(0)
    expect(DECO_IDLE_SWAY_HZ_MAX).toBe(1.5)
    expect(DECO_IDLE_SWAY_HZ_DEFAULT).toBe(0.8)
    expect(DECO_IDLE_SWAY_AMP_MIN).toBe(0)
    expect(DECO_IDLE_SWAY_AMP_MAX).toBe(40)
    expect(DECO_IDLE_SWAY_AMP_DEFAULT).toBe(20)
    // 轻晃的两个下限是 0（合法的"关掉轻晃"），不是"缺省回默认"。
    expect(DECO_IDLE_SWAY_HZ_MIN).toBe(0)
    expect(DECO_IDLE_SWAY_AMP_MIN).toBe(0)

    // 票 #74：帧率上限。
    expect(DECO_FPS_CAP_MIN).toBe(30)
    expect(DECO_FPS_CAP_MAX).toBe(160)
    expect(DECO_FPS_CAP_DEFAULT).toBe(60)

    // 票 #75：指针端偏移。下限是负数 —— 允许把绳头挂到光标左上。
    expect(DECO_POINTER_OFFSET_X_MIN).toBe(-24)
    expect(DECO_POINTER_OFFSET_X_MAX).toBe(24)
    expect(DECO_POINTER_OFFSET_X_DEFAULT).toBe(3)
    expect(DECO_POINTER_OFFSET_Y_MIN).toBe(-24)
    expect(DECO_POINTER_OFFSET_Y_MAX).toBe(24)
    expect(DECO_POINTER_OFFSET_Y_DEFAULT).toBe(3)
    expect(DECO_POINTER_OFFSET_X_MIN).toBeLessThan(0)
    expect(DECO_POINTER_OFFSET_Y_MIN).toBeLessThan(0)
  })

  it('参数清单就是表单要渲染的十二行（开关不是数值参数）', () => {
    expect(DECO_NUMBER_PARAMS.map((param) => param.key)).toEqual([
      'logo_size',
      'rope_length',
      'segments',
      'line_width',
      'gravity',
      'damping',
      'stiffness',
      'idle_sway_hz',
      'idle_sway_amp',
      'fps_cap',
      'pointer_offset_x',
      'pointer_offset_y',
    ])
  })

  it('每个参数的元数据区间与常量同源（表单不自己写数字）', () => {
    const expected = {
      logo_size: { min: DECO_LOGO_SIZE_MIN, max: DECO_LOGO_SIZE_MAX, defaultValue: DECO_LOGO_SIZE_DEFAULT },
      rope_length: { min: DECO_ROPE_LENGTH_MIN, max: DECO_ROPE_LENGTH_MAX, defaultValue: DECO_ROPE_LENGTH_DEFAULT },
      segments: { min: DECO_SEGMENTS_MIN, max: DECO_SEGMENTS_MAX, defaultValue: DECO_SEGMENTS_DEFAULT },
      line_width: { min: DECO_LINE_WIDTH_MIN, max: DECO_LINE_WIDTH_MAX, defaultValue: DECO_LINE_WIDTH_DEFAULT },
      gravity: { min: DECO_GRAVITY_MIN, max: DECO_GRAVITY_MAX, defaultValue: DECO_GRAVITY_DEFAULT },
      damping: { min: DECO_DAMPING_MIN, max: DECO_DAMPING_MAX, defaultValue: DECO_DAMPING_DEFAULT },
      stiffness: {
        min: DECO_STIFFNESS_MIN,
        max: DECO_STIFFNESS_MAX,
        defaultValue: DECO_STIFFNESS_DEFAULT,
      },
      idle_sway_hz: { min: DECO_IDLE_SWAY_HZ_MIN, max: DECO_IDLE_SWAY_HZ_MAX, defaultValue: DECO_IDLE_SWAY_HZ_DEFAULT },
      idle_sway_amp: { min: DECO_IDLE_SWAY_AMP_MIN, max: DECO_IDLE_SWAY_AMP_MAX, defaultValue: DECO_IDLE_SWAY_AMP_DEFAULT },
      fps_cap: { min: DECO_FPS_CAP_MIN, max: DECO_FPS_CAP_MAX, defaultValue: DECO_FPS_CAP_DEFAULT },
      pointer_offset_x: {
        min: DECO_POINTER_OFFSET_X_MIN,
        max: DECO_POINTER_OFFSET_X_MAX,
        defaultValue: DECO_POINTER_OFFSET_X_DEFAULT,
      },
      pointer_offset_y: {
        min: DECO_POINTER_OFFSET_Y_MIN,
        max: DECO_POINTER_OFFSET_Y_MAX,
        defaultValue: DECO_POINTER_OFFSET_Y_DEFAULT,
      },
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

  it('节点数、帧率上限、指针端偏移是整数：精度声明与量纲匹配', () => {
    const segments = DECO_NUMBER_PARAMS.find((param) => param.key === 'segments')
    const lineWidth = DECO_NUMBER_PARAMS.find((param) => param.key === 'line_width')
    const fpsCap = DECO_NUMBER_PARAMS.find((param) => param.key === 'fps_cap')
    const offsetX = DECO_NUMBER_PARAMS.find((param) => param.key === 'pointer_offset_x')
    const offsetY = DECO_NUMBER_PARAMS.find((param) => param.key === 'pointer_offset_y')

    expect(segments?.integer).toBe(true)
    expect(segments?.precision).toBe(0)
    expect(fpsCap?.integer).toBe(true)
    expect(fpsCap?.precision).toBe(0)
    expect(fpsCap?.step).toBe(1)
    expect(lineWidth?.step).toBe(0.1)
    expect(lineWidth?.precision).toBe(1)
    // 票 #75：偏移是整数 px（`NInputNumber` 的小数位锁成 0）。
    expect(offsetX?.integer).toBe(true)
    expect(offsetX?.precision).toBe(0)
    expect(offsetX?.step).toBe(1)
    expect(offsetY?.integer).toBe(true)
    expect(offsetY?.precision).toBe(0)
    expect(offsetY?.step).toBe(1)
    // 其余参数允许小数（阻尼必须能填 0.97、刚度必须能填 0.55）。
    expect(DECO_NUMBER_PARAMS.find((param) => param.key === 'damping')?.integer).toBeFalsy()
    expect(DECO_NUMBER_PARAMS.find((param) => param.key === 'stiffness')?.integer).toBeFalsy()
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

describe('票 #42 的三个新参数钳位', () => {
  it('描边宽度：越界钳到 1/6 并保留一位小数，缺省与非法回落 2.5', () => {
    expect(clampDecoLineWidth(10)).toBe(DECO_LINE_WIDTH_MAX)
    expect(clampDecoLineWidth(0.2)).toBe(DECO_LINE_WIDTH_MIN)
    expect(clampDecoLineWidth(2.54)).toBe(2.5)
    expect(clampDecoLineWidth(2.56)).toBe(2.6)
    expect(clampDecoLineWidth(2.5)).toBe(2.5)
    expect(clampDecoLineWidth(0)).toBe(DECO_LINE_WIDTH_DEFAULT)
    expect(clampDecoLineWidth(-1)).toBe(DECO_LINE_WIDTH_DEFAULT)
    expect(clampDecoLineWidth(Number.NaN)).toBe(DECO_LINE_WIDTH_DEFAULT)
  })

  it('静止摆动频率：0 原样保留（= 关掉轻晃），越界钳到 1.5，负数/NaN 回落 0.8', () => {
    expect(clampDecoIdleSwayHz(0)).toBe(0)
    expect(clampDecoIdleSwayHz(9)).toBe(DECO_IDLE_SWAY_HZ_MAX)
    expect(clampDecoIdleSwayHz(0.8)).toBe(0.8)
    expect(clampDecoIdleSwayHz(-1)).toBe(DECO_IDLE_SWAY_HZ_DEFAULT)
    expect(clampDecoIdleSwayHz(Number.NaN)).toBe(DECO_IDLE_SWAY_HZ_DEFAULT)
  })

  it('静止摆动幅度：0 原样保留（= 停帧），越界钳到 40，负数/NaN 回落 20', () => {
    expect(clampDecoIdleSwayAmp(0)).toBe(0)
    expect(clampDecoIdleSwayAmp(400)).toBe(DECO_IDLE_SWAY_AMP_MAX)
    expect(clampDecoIdleSwayAmp(20)).toBe(20)
    expect(clampDecoIdleSwayAmp(-5)).toBe(DECO_IDLE_SWAY_AMP_DEFAULT)
    expect(clampDecoIdleSwayAmp(Number.NaN)).toBe(DECO_IDLE_SWAY_AMP_DEFAULT)
  })
})

describe('票 #74 的帧率上限钳位', () => {
  it('帧率上限：越界钳到 30/160，零与非法回落 60，并取整成整数', () => {
    // 顺序是"先夹边界、再取整"：160.6 必须得到 160，而不是 161。
    expect(clampDecoFpsCap(29)).toBe(DECO_FPS_CAP_MIN)
    expect(clampDecoFpsCap(30.4)).toBe(DECO_FPS_CAP_MIN)
    expect(clampDecoFpsCap(161)).toBe(DECO_FPS_CAP_MAX)
    expect(clampDecoFpsCap(160.6)).toBe(DECO_FPS_CAP_MAX)
    expect(clampDecoFpsCap(90)).toBe(90)
    expect(clampDecoFpsCap(90.4)).toBe(90)
    expect(clampDecoFpsCap(90.6)).toBe(91)
    expect(clampDecoFpsCap(0)).toBe(DECO_FPS_CAP_DEFAULT)
    expect(clampDecoFpsCap(-30)).toBe(DECO_FPS_CAP_DEFAULT)
    expect(clampDecoFpsCap(Number.NaN)).toBe(DECO_FPS_CAP_DEFAULT)
  })
})

describe('票 #75 的指针端偏移钳位（第四条规则：允许负数的区间）', () => {
  /** X / Y 两轴共用同一张表：区间、取整与回落都必须完全对称。 */
  const SIGNED_CASES: { input: number; expectedX: number; expectedY: number; note: string }[] = [
    { input: 3, expectedX: 3, expectedY: 3, note: '默认值原样保留' },
    { input: 0, expectedX: 0, expectedY: 0, note: '0 是普通值，不回落默认' },
    { input: 12, expectedX: 12, expectedY: 12, note: '区间内正值原样保留' },
    { input: -6, expectedX: -6, expectedY: -6, note: '负值原样保留（往左上偏）' },
    { input: 24, expectedX: 24, expectedY: 24, note: '上限本身不漂移' },
    { input: -24, expectedX: -24, expectedY: -24, note: '下限本身不漂移' },
    { input: 24.4, expectedX: 24, expectedY: 24, note: '先夹边界再取整（上）' },
    { input: -24.4, expectedX: -24, expectedY: -24, note: '先夹边界再取整（下）' },
    { input: 100, expectedX: 24, expectedY: 24, note: '远超上限钳到边界' },
    { input: -100, expectedX: -24, expectedY: -24, note: '远超下限钳到边界' },
    { input: 3.7, expectedX: 4, expectedY: 4, note: '取整（向上）' },
    { input: -3.7, expectedX: -4, expectedY: -4, note: '负数取整（远离 0）' },
    // 负半值必须与 Go 的 math.Round 同口径（远离 0）；直接写 Math.round 会得到
    // -2，与后端分叉 —— 共享夹具里也钉着这一条。
    { input: -2.5, expectedX: -3, expectedY: -3, note: '负半值远离 0' },
  ]

  it.each(SIGNED_CASES)('$note：输入 $input', ({ input, expectedX, expectedY }) => {
    expect(clampDecoPointerOffsetX(input), `X 轴输入 ${input}`).toBe(expectedX)
    expect(clampDecoPointerOffsetY(input), `Y 轴输入 ${input}`).toBe(expectedY)
  })

  it('NaN / ±Inf 回落默认值 3（两者序列化进 JSON 会直接失败）', () => {
    for (const bad of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
      expect(clampDecoPointerOffsetX(bad)).toBe(DECO_POINTER_OFFSET_X_DEFAULT)
      expect(clampDecoPointerOffsetY(bad)).toBe(DECO_POINTER_OFFSET_Y_DEFAULT)
    }
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

describe('clampDecoStiffness', () => {
  it('刚度：越界钳到 0.2/1，零与非法值回落 0.5（0 是缺省不是最软）', () => {
    expect(clampDecoStiffness(0.5)).toBe(0.5)
    expect(clampDecoStiffness(0.1)).toBe(DECO_STIFFNESS_MIN)
    expect(clampDecoStiffness(2)).toBe(DECO_STIFFNESS_MAX)
    expect(clampDecoStiffness(0)).toBe(DECO_STIFFNESS_DEFAULT)
    expect(clampDecoStiffness(-0.1)).toBe(DECO_STIFFNESS_DEFAULT)
    expect(clampDecoStiffness(Number.NaN)).toBe(DECO_STIFFNESS_DEFAULT)
    expect(clampDecoStiffness(Number.POSITIVE_INFINITY)).toBe(DECO_STIFFNESS_DEFAULT)
  })
})

describe('恢复默认（票 #42 验收项）', () => {
  it('恰好是规范里的十三个默认值（开关 + 十二个数值）', () => {
    expect(defaultDecoConfig()).toEqual({
      charm: {
        enable: true,
        logo_size: DECO_LOGO_SIZE_DEFAULT,
        rope_length: DECO_ROPE_LENGTH_DEFAULT,
        segments: DECO_SEGMENTS_DEFAULT,
        line_width: DECO_LINE_WIDTH_DEFAULT,
        gravity: DECO_GRAVITY_DEFAULT,
        damping: DECO_DAMPING_DEFAULT,
        stiffness: DECO_STIFFNESS_DEFAULT,
        idle_sway_hz: DECO_IDLE_SWAY_HZ_DEFAULT,
        idle_sway_amp: DECO_IDLE_SWAY_AMP_DEFAULT,
        fps_cap: DECO_FPS_CAP_DEFAULT,
        pointer_offset_x: DECO_POINTER_OFFSET_X_DEFAULT,
        pointer_offset_y: DECO_POINTER_OFFSET_Y_DEFAULT,
      },
    })
  })

  it('每个默认值都落在自己的合法区间内（恢复默认不会被钳位改动）', () => {
    const defaults = defaultDecoConfig().charm
    expect(defaults.logo_size).toBeGreaterThanOrEqual(DECO_LOGO_SIZE_MIN)
    expect(defaults.logo_size).toBeLessThanOrEqual(DECO_LOGO_SIZE_MAX)
    expect(defaults.rope_length).toBeGreaterThanOrEqual(DECO_ROPE_LENGTH_MIN)
    expect(defaults.rope_length).toBeLessThanOrEqual(DECO_ROPE_LENGTH_MAX)
    expect(defaults.segments).toBeGreaterThanOrEqual(DECO_SEGMENTS_MIN)
    expect(defaults.segments).toBeLessThanOrEqual(DECO_SEGMENTS_MAX)
    expect(defaults.line_width).toBeGreaterThanOrEqual(DECO_LINE_WIDTH_MIN)
    expect(defaults.line_width).toBeLessThanOrEqual(DECO_LINE_WIDTH_MAX)
    expect(defaults.gravity).toBeGreaterThanOrEqual(DECO_GRAVITY_MIN)
    expect(defaults.gravity).toBeLessThanOrEqual(DECO_GRAVITY_MAX)
    expect(defaults.damping).toBeGreaterThanOrEqual(DECO_DAMPING_MIN)
    expect(defaults.damping).toBeLessThanOrEqual(DECO_DAMPING_MAX)
    expect(defaults.stiffness).toBeGreaterThanOrEqual(DECO_STIFFNESS_MIN)
    expect(defaults.stiffness).toBeLessThanOrEqual(DECO_STIFFNESS_MAX)
    expect(defaults.idle_sway_hz).toBeGreaterThanOrEqual(DECO_IDLE_SWAY_HZ_MIN)
    expect(defaults.idle_sway_hz).toBeLessThanOrEqual(DECO_IDLE_SWAY_HZ_MAX)
    expect(defaults.idle_sway_amp).toBeGreaterThanOrEqual(DECO_IDLE_SWAY_AMP_MIN)
    expect(defaults.idle_sway_amp).toBeLessThanOrEqual(DECO_IDLE_SWAY_AMP_MAX)
    expect(defaults.fps_cap).toBeGreaterThanOrEqual(DECO_FPS_CAP_MIN)
    expect(defaults.fps_cap).toBeLessThanOrEqual(DECO_FPS_CAP_MAX)
    expect(defaults.pointer_offset_x).toBeGreaterThanOrEqual(DECO_POINTER_OFFSET_X_MIN)
    expect(defaults.pointer_offset_x).toBeLessThanOrEqual(DECO_POINTER_OFFSET_X_MAX)
    expect(defaults.pointer_offset_y).toBeGreaterThanOrEqual(DECO_POINTER_OFFSET_Y_MIN)
    expect(defaults.pointer_offset_y).toBeLessThanOrEqual(DECO_POINTER_OFFSET_Y_MAX)
    // 恢复默认的结果本身也必须能通过归一化（幂等）。
    expect(normalizeDecoConfig(defaultDecoConfig())).toEqual(defaultDecoConfig())
  })
})

describe('normalizeDecoConfig', () => {
  it('缺子块时补默认值（开关默认开、十二个数值都取默认）', () => {
    expect(normalizeDecoConfig({} as never)).toEqual(defaultDecoConfig())
  })

  it('保留管理员显式关掉的开关，同时钳住全部十二个数值', () => {
    expect(
      normalizeDecoConfig({
        charm: {
          enable: false,
          logo_size: 999,
          rope_length: 9999,
          segments: 100,
          line_width: 9,
          gravity: 99999,
          damping: 2,
          stiffness: 2,
          idle_sway_hz: 9,
          idle_sway_amp: 999,
          fps_cap: 999,
          pointer_offset_x: 999,
          pointer_offset_y: -999,
        },
      }),
    ).toEqual({
      charm: {
        enable: false,
        logo_size: DECO_LOGO_SIZE_MAX,
        rope_length: DECO_ROPE_LENGTH_MAX,
        segments: DECO_SEGMENTS_MAX,
        line_width: DECO_LINE_WIDTH_MAX,
        gravity: DECO_GRAVITY_MAX,
        damping: DECO_DAMPING_MAX,
        stiffness: DECO_STIFFNESS_MAX,
        idle_sway_hz: DECO_IDLE_SWAY_HZ_MAX,
        idle_sway_amp: DECO_IDLE_SWAY_AMP_MAX,
        fps_cap: DECO_FPS_CAP_MAX,
        pointer_offset_x: DECO_POINTER_OFFSET_X_MAX,
        pointer_offset_y: DECO_POINTER_OFFSET_Y_MIN,
      },
    })
  })

  it('部分字段缺失只影响它自己；轻晃填 0 时保留 0（不回落默认）', () => {
    expect(
      normalizeDecoConfig({ charm: { enable: true, rope_length: 80, idle_sway_hz: 0, idle_sway_amp: 0 } })
        .charm,
    ).toEqual({
      enable: true,
      logo_size: DECO_LOGO_SIZE_DEFAULT,
      rope_length: 80,
      segments: DECO_SEGMENTS_DEFAULT,
      line_width: DECO_LINE_WIDTH_DEFAULT,
      gravity: DECO_GRAVITY_DEFAULT,
      damping: DECO_DAMPING_DEFAULT,
      stiffness: DECO_STIFFNESS_DEFAULT,
      idle_sway_hz: 0,
      idle_sway_amp: 0,
      fps_cap: DECO_FPS_CAP_DEFAULT,
      pointer_offset_x: DECO_POINTER_OFFSET_X_DEFAULT,
      pointer_offset_y: DECO_POINTER_OFFSET_Y_DEFAULT,
    })
  })

  it('负偏移穿过归一化后仍然为负；0 偏移仍然是 0（都不是回落默认）', () => {
    const negative = normalizeDecoConfig({
      charm: { pointer_offset_x: -6, pointer_offset_y: -12 },
    }).charm
    expect(negative.pointer_offset_x).toBe(-6)
    expect(negative.pointer_offset_y).toBe(-12)

    const zero = normalizeDecoConfig({ charm: { pointer_offset_x: 0, pointer_offset_y: 0 } }).charm
    expect(zero.pointer_offset_x).toBe(0)
    expect(zero.pointer_offset_y).toBe(0)
  })

  it('提交出去的形状只有 charm 子块（不夹带未知字段）', () => {
    const normalized = normalizeDecoConfig({
      charm: { enable: true, logo_size: 30 },
      extra: 'x',
    } as never)
    expect(Object.keys(normalized)).toEqual(['charm'])
  })
})

/**
 * 票 #77：`null` 与 `undefined` 一样表示「这个键没有值」，必须回落默认值。
 *
 * 为什么后台也要管：后台读回来的是一段自由 JSON（旧库、旧版本、手改的配置都
 * 可能带 null），而它的归一化结果会被原样提交回后端。若 `Number(null) === 0`
 * 在这里被当成合法值，"清空字段"就会变成"明确填 0" —— 轻晃被关掉、指针端压到
 * 热点上，管理员在表单里看到的却还是默认数字。归一化是唯一兜底点。
 */
describe('票 #77：null 与 undefined 同义（都表示这个键没有值）', () => {
  it('四个「0 合法」数值字段为 null 时回落默认值，而不是被读成 0', () => {
    const charm = normalizeDecoConfig({
      charm: {
        idle_sway_hz: null,
        idle_sway_amp: null,
        pointer_offset_x: null,
        pointer_offset_y: null,
      },
    }).charm

    expect(charm.idle_sway_hz).toBe(DECO_IDLE_SWAY_HZ_DEFAULT)
    expect(charm.idle_sway_amp).toBe(DECO_IDLE_SWAY_AMP_DEFAULT)
    expect(charm.pointer_offset_x).toBe(DECO_POINTER_OFFSET_X_DEFAULT)
    expect(charm.pointer_offset_y).toBe(DECO_POINTER_OFFSET_Y_DEFAULT)
  })

  it('正数项（重力）为 null 时同样回落默认值，而不是 0', () => {
    const charm = normalizeDecoConfig({ charm: { gravity: null } }).charm

    expect(charm.gravity).toBe(DECO_GRAVITY_DEFAULT)
  })

  it('显式 0 仍然是 0（null 的回落不吃掉「关掉轻晃 / 压在热点上」）', () => {
    const charm = normalizeDecoConfig({
      charm: { idle_sway_hz: 0, idle_sway_amp: 0, pointer_offset_x: 0, pointer_offset_y: 0 },
    }).charm

    expect(charm.idle_sway_hz).toBe(0)
    expect(charm.idle_sway_amp).toBe(0)
    expect(charm.pointer_offset_x).toBe(0)
    expect(charm.pointer_offset_y).toBe(0)
  })

  it('undefined 与 null 的结果逐字段一致（十二个数值项全 null）', () => {
    const keys = DECO_NUMBER_PARAMS.map((param) => param.key)
    const withNull = normalizeDecoConfig({
      charm: Object.fromEntries(keys.map((key) => [key, null])),
    })
    const withUndefined = normalizeDecoConfig({
      charm: Object.fromEntries(keys.map((key) => [key, undefined])),
    })

    expect(withNull).toEqual(withUndefined)
    expect(withNull).toEqual(defaultDecoConfig())
  })
})

/** 共享归一化用例表（后端 conf_deco_test.go 与读者站 decoCharm.test.ts 也读同一份）。 */
const FIXTURE_RELATIVE_PATH = 'backend/conf/testdata/deco-charm-normalization.json'

interface NormalizationCase {
  parameter: string
  input: number | string
  expected: number
  reason: string
}

function loadNormalizationCases(): NormalizationCase[] {
  const fixture = JSON.parse(readFileSync(join(process.cwd(), '..', FIXTURE_RELATIVE_PATH), 'utf8')) as {
    cases: NormalizationCase[]
  }
  return fixture.cases
}

/** JSON 没有 NaN 字面量：夹具用字符串表达，三端各自映射成同一个数值。 */
function fixtureInput(input: number | string): number {
  if (typeof input === 'number') return input
  if (input === 'NaN') return Number.NaN
  if (input === 'Infinity') return Number.POSITIVE_INFINITY
  if (input === '-Infinity') return Number.NEGATIVE_INFINITY
  throw new Error(`共享夹具里出现了不认识的 input: ${JSON.stringify(input)}`)
}

/** 参数键 → 后台的钳位函数（夹具按后端 snake_case 命名）。 */
const DECO_CLAMPS: Record<string, (value: number) => number> = {
  logo_size: clampDecoLogoSize,
  rope_length: clampDecoRopeLength,
  segments: clampDecoSegments,
  line_width: clampDecoLineWidth,
  gravity: clampDecoGravity,
  damping: clampDecoDamping,
  stiffness: clampDecoStiffness,
  idle_sway_hz: clampDecoIdleSwayHz,
  idle_sway_amp: clampDecoIdleSwayAmp,
  fps_cap: clampDecoFpsCap,
  pointer_offset_x: clampDecoPointerOffsetX,
  pointer_offset_y: clampDecoPointerOffsetY,
}

describe('共享归一化用例表：后台与后端 / 读者站同一组输入同一组输出', () => {
  it('夹具真的读到了，且覆盖全部十二个数值参数', () => {
    const cases = loadNormalizationCases()

    expect(cases.length).toBeGreaterThan(40)
    for (const parameter of Object.keys(DECO_CLAMPS)) {
      expect(cases.some((testCase) => testCase.parameter === parameter), parameter).toBe(true)
    }
  })

  const cases = loadNormalizationCases()
  it.each(cases)(
    '$parameter 输入 $input → $expected（$reason）',
    (testCase: NormalizationCase) => {
      const clamp = DECO_CLAMPS[testCase.parameter]
      expect(clamp, `夹具里的参数 ${testCase.parameter} 没有对应的后台钳位函数`).toBeDefined()
      expect(clamp!(fixtureInput(testCase.input))).toBe(testCase.expected)
    },
  )
})
