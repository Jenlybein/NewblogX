import { describe, expect, it } from 'vitest'
import {
  LINE_CHART_THRESHOLD,
  aiResultLabel,
  barHeightPercent,
  buildLinePoints,
  maxTokens,
  pickChartType,
  sceneLabel,
  shortTokens,
} from '~/utils/aiTokens'

describe('pickChartType', () => {
  it('20 个点及以内用柱状（本周 7、本年 12）', () => {
    expect(pickChartType(0)).toBe('bar')
    expect(pickChartType(1)).toBe('bar')
    expect(pickChartType(7)).toBe('bar')
    expect(pickChartType(12)).toBe('bar')
    expect(pickChartType(LINE_CHART_THRESHOLD)).toBe('bar')
  })

  it('超过 20 个点用折线（今日 24、本月 28~31）', () => {
    expect(pickChartType(21)).toBe('line')
    expect(pickChartType(24)).toBe('line')
    expect(pickChartType(31)).toBe('line')
  })
})

describe('maxTokens', () => {
  it('空序列返回 1，避免除零', () => {
    expect(maxTokens([])).toBe(1)
  })

  it('取最大值', () => {
    expect(maxTokens([3, 9, 2])).toBe(9)
  })

  it('全为零时返回 1', () => {
    expect(maxTokens([0, 0])).toBe(1)
  })

  it('忽略非法值，只看真实数字', () => {
    expect(maxTokens([Number.NaN, 5, Number.POSITIVE_INFINITY])).toBe(5)
  })
})

describe('barHeightPercent', () => {
  it('0 和无数据留 2px', () => {
    expect(barHeightPercent(0, 100)).toBe('2px')
    expect(barHeightPercent(-5, 100)).toBe('2px')
    expect(barHeightPercent(Number.NaN, 100)).toBe('2px')
  })

  it('按最大值折算百分比', () => {
    expect(barHeightPercent(50, 100)).toBe('50%')
    expect(barHeightPercent(100, 100)).toBe('100%')
  })

  it('极小值保底 8%，否则柱子看不见', () => {
    expect(barHeightPercent(1, 10000)).toBe('8%')
  })

  it('max 非法时按 1 处理，不产生 Infinity', () => {
    expect(barHeightPercent(5, 0)).toBe('100%')
    expect(barHeightPercent(5, Number.NaN)).toBe('100%')
  })
})

describe('buildLinePoints', () => {
  it('空序列返回空串', () => {
    expect(buildLinePoints([])).toBe('')
  })

  it('单点给水平中线，避免图上什么都没有', () => {
    expect(buildLinePoints([42])).toBe('0,50')
  })

  it('两点按 x 均分、y 按最大值折算', () => {
    expect(buildLinePoints([0, 10])).toBe('0,100 100,0')
  })

  it('三点等距分布', () => {
    expect(buildLinePoints([0, 10, 0])).toBe('0,100 50,0 100,100')
  })

  it('负值按 0 处理，不跑到图外', () => {
    expect(buildLinePoints([-5, 10])).toBe('0,100 100,0')
  })
})

describe('shortTokens', () => {
  it('小于 1000 原样显示', () => {
    expect(shortTokens(0)).toBe('0')
    expect(shortTokens(999)).toBe('999')
  })

  it('千级用 k', () => {
    expect(shortTokens(1200)).toBe('1.2k')
    expect(shortTokens(9999)).toBe('10.0k')
  })

  it('万级用万', () => {
    expect(shortTokens(12000)).toBe('1.2万')
    expect(shortTokens(1234567)).toBe('123.5万')
  })

  it('亿级用亿', () => {
    expect(shortTokens(123_456_789)).toBe('1.2亿')
  })

  it('非法值按 0 处理', () => {
    expect(shortTokens(-1)).toBe('0')
    expect(shortTokens(Number.NaN)).toBe('0')
  })
})

describe('sceneLabel', () => {
  it('已知场景转中文', () => {
    expect(sceneLabel('scoring')).toBe('文章评分')
    expect(sceneLabel('search_answer')).toBe('搜索问答')
  })

  it('空值显示 -', () => {
    expect(sceneLabel('')).toBe('-')
    expect(sceneLabel('   ')).toBe('-')
  })

  it('未知场景原样返回，不吞信息', () => {
    expect(sceneLabel('brand_new_scene')).toBe('brand_new_scene')
  })
})

describe('aiResultLabel', () => {
  it('1 是成功，其余是失败', () => {
    expect(aiResultLabel(1)).toBe('成功')
    expect(aiResultLabel(0)).toBe('失败')
  })
})
