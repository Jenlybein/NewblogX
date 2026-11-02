import { describe, expect, it } from 'vitest'

import {
  DECO_DAMPING_DEFAULT,
  DECO_FPS_CAP_DEFAULT,
  DECO_GRAVITY_DEFAULT,
  DECO_IDLE_SWAY_AMP_DEFAULT,
  DECO_IDLE_SWAY_HZ_DEFAULT,
  DECO_LINE_WIDTH_DEFAULT,
  DECO_LOGO_SIZE_DEFAULT,
  DECO_NUMBER_PARAMS,
  DECO_POINTER_OFFSET_X_DEFAULT,
  DECO_POINTER_OFFSET_Y_DEFAULT,
  DECO_ROPE_LENGTH_DEFAULT,
  DECO_SEGMENTS_DEFAULT,
  DECO_STIFFNESS_DEFAULT,
  defaultDecoConfig,
  normalizeDecoConfig,
  type DecoCharmNumericKey,
} from '~/utils/decoParams'

/**
 * 票 #78 的一致性守卫（后台）：参数清单 ↔ 类型 ↔ 默认值 ↔ 归一化必须是同一份。
 *
 * 为什么值得一条独立守卫：票面把后台的 `(min, max, default)` 从"四份手抄"收敛成
 * "元数据一份、其余派生"。派生本身是编译期的事，`vitest` 不跑类型检查，所以
 * **漏一行元数据 / 归一化跳过某个键**这类错误只能靠运行时断言抓住：
 *   - `DECO_NUMBER_PARAMS`（表单与归一化的唯一清单）；
 *   - `DecoCharmConfig` 的数值键（由元数据派生的类型，运行时用下面这张常量表枚举）；
 *   - `normalizeDecoConfig` / `defaultDecoConfig` 实际输出的键；
 * 三者必须完全相等，且每个参数的 `defaultValue` 必须等于对应的
 * `DECO_*_DEFAULT` 常量（元数据不许自己写数字）。
 */

/**
 * 每个数值键对应的 `DECO_*_DEFAULT` 常量（独立来源：具名常量，不是元数据）。
 *
 * `satisfies Record<DecoCharmNumericKey, number>` 同时是一条**类型层断言**：
 * 它要求这里恰好列出 `DecoCharmConfig` 的全部数值键。元数据里删掉一行
 * （派生的键集合少一个）会让这里多出一个键、`tsc` 先红；反过来类型多一个字段
 * 而这里没跟上，也会红。运行时再用 `Object.keys` 与元数据 / 归一化输出比对。
 */
const DEFAULT_CONSTANTS = {
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
} as const satisfies Record<DecoCharmNumericKey, number>

/** 一份输出里除总开关以外的数值键（`enable` 不是数值参数）。 */
function numericKeys(charm: object): string[] {
  return Object.keys(charm)
    .filter((key) => key !== 'enable')
    .sort()
}

describe('参数清单一致性守卫（#78）', () => {
  it('元数据 key 集合 == 类型数值键集合 == 归一化 / 恢复默认输出的键集合', () => {
    const metadataKeys = DECO_NUMBER_PARAMS.map((param) => param.key).sort()
    const typeKeys = Object.keys(DEFAULT_CONSTANTS).sort()

    expect(metadataKeys, '元数据清单与类型的数值键集合不一致').toEqual(typeKeys)
    expect(numericKeys(normalizeDecoConfig(null).charm), '归一化漏了某个参数（或凭空多出一个）').toEqual(
      typeKeys,
    )
    expect(numericKeys(defaultDecoConfig().charm), '恢复默认漏了某个参数（或凭空多出一个）').toEqual(
      typeKeys,
    )
  })

  it('每个参数的 defaultValue 等于对应的 DECO_*_DEFAULT 常量，且落在自己的区间内', () => {
    for (const param of DECO_NUMBER_PARAMS) {
      // 元数据里若手写了数字（而不是引用常量），这条会红。
      expect(param.defaultValue, `deco.charm.${param.key} 的 defaultValue 与常量不一致`).toBe(
        DEFAULT_CONSTANTS[param.key],
      )
      expect(param.defaultValue).toBeGreaterThanOrEqual(param.min)
      expect(param.defaultValue).toBeLessThanOrEqual(param.max)
    }
  })

  it('归一化按元数据的区间逐项钳位（不是某几个手写字段）', () => {
    for (const param of DECO_NUMBER_PARAMS) {
      const normalized = normalizeDecoConfig({ charm: { [param.key]: 1e9 } }).charm

      // 喂一个远大于任何上限的值，每个参数都必须钳到**自己的**上限；
      // 归一化若跳过某个键（返回值是 undefined）或用了别个参数的区间，这里立刻红。
      expect(normalized[param.key], `deco.charm.${param.key} 没有被归一化`).toBe(param.max)
    }
  })
})
