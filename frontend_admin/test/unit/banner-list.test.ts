import { describe, expect, it } from 'vitest'
import { buildBannerListQuery } from '~/utils/bannerList'

describe('buildBannerListQuery', () => {
  it.each([
    ['all', { page: 2, limit: 10 }],
    ['shown', { page: 2, limit: 10, show: true }],
    ['hidden', { page: 2, limit: 10, show: false }],
  ] as const)('%s 映射成后端三态查询', (filter, expected) => {
    expect(buildBannerListQuery(2, 10, filter)).toEqual(expected)
  })
})
