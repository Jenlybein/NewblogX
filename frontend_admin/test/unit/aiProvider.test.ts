import { describe, expect, it } from 'vitest'
import { mergeVendorFields, vendorFieldDefaults } from '~/utils/aiProvider'
import type { VendorItem } from '~/types/api'

const openaiVendor: VendorItem = {
  id: 'openai',
  name: 'OpenAI',
  note: '',
  fields: [
    { key: 'api_key', label: 'API Key', required: true, secret: true, placeholder: 'sk-...', default: '' },
    { key: 'base_url', label: '接口地址', required: false, secret: false, placeholder: '', default: 'https://api.openai.com/v1' },
    { key: 'model', label: '模型', required: true, secret: false, placeholder: 'gpt-4.1-mini', default: '' },
  ],
}

describe('vendorFieldDefaults', () => {
  it('按字段定义铺默认值，没有默认值的填空串', () => {
    expect(vendorFieldDefaults(openaiVendor)).toEqual({
      api_key: '',
      base_url: 'https://api.openai.com/v1',
      model: '',
    })
  })

  it('供应商为空时返回空对象，不炸', () => {
    expect(vendorFieldDefaults(null)).toEqual({})
    expect(vendorFieldDefaults(undefined)).toEqual({})
  })
})

describe('mergeVendorFields', () => {
  it('已有值优先，缺的用默认值补齐', () => {
    const merged = mergeVendorFields(openaiVendor, { api_key: '******', base_url: 'https://proxy.example.com/v1' })
    expect(merged.api_key).toBe('******')
    expect(merged.base_url).toBe('https://proxy.example.com/v1')
    expect(merged.model).toBe('')
  })

  it('current 为空时等价于纯默认值', () => {
    expect(mergeVendorFields(openaiVendor, undefined)).toEqual(vendorFieldDefaults(openaiVendor))
  })
})
