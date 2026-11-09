import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import BannerFormModal from '~/components/media/BannerFormModal.vue'
import type { BannerModel } from '~/types/api'

/**
 * 新增 / 编辑轮播的同一张表单卡（#330）。
 *
 * 用户的要求是两件事：新增收成一个按钮 + 弹窗（不铺满全屏）、编辑复用同一张卡
 * 并且能改标题 / 跳转地址 / 封面。这一层只锁**会真出错**的行为：
 *
 *   1. 新建时不带封面就提交 → 拦住、不发请求（否则会留下一条没有封面的轮播）；
 *   2. 编辑时**不换封面也能只改标题** → 沿用该行原有 `cover_image_id`，不逼人重传；
 *   3. 编辑时该行缺 `cover_image_id`（历史 / 数据损坏）→ 明确要求换封面，不发坏请求；
 *   4. 更新是整体替换 → 提交的请求体必须同时带上标题 / 跳转地址 / 展示；
 *   5. 两种模式的标题与按钮文案不同，字段完全一致（这就是"复用同一张卡"）。
 *
 * 封面定形 / 裁剪那套留在 `useImageCropUpload` 与 `imageShaping` 的既有用例里，
 * 这里不重复模拟画布。
 */

const { messageMock, adminApi, uploadMock } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  adminApi: { createBanner: vi.fn(), updateBanner: vi.fn() },
  uploadMock: vi.fn(),
}))

vi.mock('~/services/admin', () => adminApi)
vi.mock('~/services/image', () => ({ uploadImageByTask: uploadMock }))

// `useMessage` 要拿假的（不在 provider 里会抛），其余 naive-ui 组件保持真实实现。
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('naive-ui')>()
  return { ...actual, useMessage: () => messageMock }
})

function bannerRow(overrides: Partial<BannerModel> = {}): BannerModel {
  return {
    id: '9',
    cover: 'https://cdn.example.cn/cover.png',
    cover_image_id: '7',
    href: '/article/42',
    title: '春季上新',
    show: true,
    created_at: '2026-01-01T00:00:00+08:00',
    updated_at: '2026-01-02T00:00:00+08:00',
    deleted_at: null,
    ...overrides,
  } as BannerModel
}

function mountModal(props: { show: boolean; mode?: 'create' | 'edit'; banner?: BannerModel | null }) {
  // NModal 内部是 Teleport：存根掉才能在同一棵树里断言弹窗内容。
  return mount(BannerFormModal, {
    props: { mode: 'create', banner: null, ...props },
    global: { stubs: { teleport: true } },
  })
}

/** 取字段对应的真实 `input`（naive-ui 的 NInput 外层是包装元素）。 */
function inputOf(wrapper: ReturnType<typeof mountModal>, role: string) {
  return wrapper.get(`[data-role="${role}"] input`)
}

beforeEach(() => {
  vi.clearAllMocks()
  adminApi.createBanner.mockResolvedValue({ id: '100' })
  adminApi.updateBanner.mockResolvedValue(undefined)
})

describe('新建模式：空表单，缺封面不发请求', () => {
  it('标题与按钮是「新增轮播 / 创建轮播」，四个字段都在', async () => {
    const wrapper = mountModal({ show: true, mode: 'create' })
    await flushPromises()

    expect(wrapper.text()).toContain('新增轮播')
    expect(wrapper.text()).toContain('创建轮播')

    // 新增与编辑共用同一组字段——这正是"复用同一张卡"的意思。
    for (const role of ['banner-form-title', 'banner-form-href', 'banner-form-show']) {
      expect(wrapper.find(`[data-role="${role}"]`).exists(), role).toBe(true)
    }
    expect(wrapper.find('[data-role="banner-form-cover"]').exists()).toBe(true)

    expect((inputOf(wrapper, 'banner-form-title').element as HTMLInputElement).value).toBe('')
    expect((inputOf(wrapper, 'banner-form-href').element as HTMLInputElement).value).toBe('')
  })

  it('没有选封面就点创建 → 报错并且一条请求都不发', async () => {
    const wrapper = mountModal({ show: true, mode: 'create' })
    await flushPromises()

    await inputOf(wrapper, 'banner-form-href').setValue('https://gentlybeing.cn/a')
    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    expect(messageMock.error).toHaveBeenCalled()
    expect(adminApi.createBanner).not.toHaveBeenCalled()
    expect(adminApi.updateBanner).not.toHaveBeenCalled()
  })
})

describe('编辑模式：同一张卡，回填该行四个字段', () => {
  it('标题 / 按钮文案改成编辑，字段带入原值', async () => {
    const wrapper = mountModal({ show: true, mode: 'edit', banner: bannerRow() })
    await flushPromises()

    expect(wrapper.text()).toContain('编辑轮播')
    expect(wrapper.text()).toContain('保存修改')

    expect((inputOf(wrapper, 'banner-form-title').element as HTMLInputElement).value).toBe('春季上新')
    expect((inputOf(wrapper, 'banner-form-href').element as HTMLInputElement).value).toBe('/article/42')
    // 原有封面直接作为预览，不必为了改标题重传一张图。
    expect(wrapper.get('[data-role="banner-form-cover"] img').attributes('src')).toBe(
      'https://cdn.example.cn/cover.png',
    )
  })

  it('不换封面也能只改标题：请求体沿用原 cover_image_id，并带上其余字段（整体替换）', async () => {
    const wrapper = mountModal({ show: true, mode: 'edit', banner: bannerRow() })
    await flushPromises()

    await inputOf(wrapper, 'banner-form-title').setValue('夏日上新')
    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    // 没有触发任何上传：改标题不该逼人重传封面。
    expect(uploadMock).not.toHaveBeenCalled()
    expect(adminApi.updateBanner).toHaveBeenCalledTimes(1)
    expect(adminApi.updateBanner).toHaveBeenCalledWith('9', {
      cover_image_id: '7',
      title: '夏日上新',
      href: '/article/42',
      show: true,
    })
    expect(adminApi.createBanner).not.toHaveBeenCalled()
  })

  it('改跳转地址与展示状态一起提交（更新是整体替换，漏一个就会清掉）', async () => {
    const wrapper = mountModal({ show: true, mode: 'edit', banner: bannerRow({ show: false }) })
    await flushPromises()

    await inputOf(wrapper, 'banner-form-href').setValue('https://gentlybeing.cn/new')
    await wrapper.get('[data-role="banner-form-show"]').trigger('click')
    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    expect(adminApi.updateBanner).toHaveBeenCalledWith('9', {
      cover_image_id: '7',
      title: '春季上新',
      href: 'https://gentlybeing.cn/new',
      show: true,
    })
  })
})

describe('编辑模式：缺封面标识的历史行必须换封面', () => {
  it('cover_image_id 缺失且没换封面 → 明确报错，不发坏请求', async () => {
    const wrapper = mountModal({
      show: true,
      mode: 'edit',
      banner: bannerRow({ cover_image_id: '', cover: '' }),
    })
    await flushPromises()

    await inputOf(wrapper, 'banner-form-title').setValue('改个标题')
    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    expect(adminApi.updateBanner).not.toHaveBeenCalled()
    expect(messageMock.error).toHaveBeenCalled()
    // 提示要说明"为什么不行"，而不是只说保存失败。
    const text = messageMock.error.mock.calls.at(-1)?.[0] as string
    expect(text).toContain('封面')
  })
})

describe('保存成功后的收尾', () => {
  it('关闭弹窗并通知父级刷新列表', async () => {
    const wrapper = mountModal({ show: true, mode: 'edit', banner: bannerRow() })
    await flushPromises()

    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:show')?.at(-1)).toEqual([false])
    expect(wrapper.emitted('saved')).toBeTruthy()
    expect(messageMock.success).toHaveBeenCalled()
  })

  it('保存失败时弹窗不关、不通知刷新（否则用户以为存上了）', async () => {
    adminApi.updateBanner.mockRejectedValueOnce(new Error('网络中断'))
    const wrapper = mountModal({ show: true, mode: 'edit', banner: bannerRow() })
    await flushPromises()

    await wrapper.get('[data-role="banner-form-submit"]').trigger('click')
    await flushPromises()

    expect(messageMock.error).toHaveBeenCalled()
    expect(wrapper.emitted('saved')).toBeFalsy()
    expect(wrapper.emitted('update:show')).toBeFalsy()
  })
})
