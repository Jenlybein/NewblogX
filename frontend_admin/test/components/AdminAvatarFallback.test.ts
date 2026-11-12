import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AdminAvatar from '~/components/admin/AdminAvatar.vue'

/**
 * 头像加载失败的回退（2026-11-13 用户反馈"有个用户头像裂开了"）。
 *
 * 以前只在 `src` 为空时才回退，地址有值但图片 404 就留一个破图标 ——
 * 比没有头像更难看。这条用例钉住"加载失败同样落到首字符"。
 */
describe('AdminAvatar：图片加载失败时的回退', () => {
  it('地址有值、加载失败 → 回退到首字符，不留破图标', async () => {
    const wrapper = mount(AdminAvatar, { props: { src: 'https://cdn.test/gone.png', name: '张三' } })

    // 起初是图片形态
    expect(wrapper.find('[data-role="admin-avatar-image"]').exists()).toBe(true)

    // naive-ui 把 on-error 透传到内部 <img>；触发它就是"图片挂了"
    const img = wrapper.find('[data-role="admin-avatar-image"] img')
    expect(img.exists()).toBe(true)
    await img.trigger('error')

    expect(wrapper.find('[data-role="admin-avatar-image"]').exists()).toBe(false)
    expect(wrapper.get('[data-role="admin-avatar-initial"]').text()).toBe('张')
  })

  it('加载失败且取不到首字符（邮箱用户名）→ 落到人形图标', async () => {
    const wrapper = mount(AdminAvatar, { props: { src: 'https://cdn.test/gone.png', name: '1813288952@qq.com' } })

    const img = wrapper.find('[data-role="admin-avatar-image"] img')
    expect(img.exists()).toBe(true)
    await img.trigger('error')

    expect(wrapper.find('[data-role="admin-avatar-image"]').exists()).toBe(false)
    expect(wrapper.find('[data-role="admin-avatar-icon"]').exists()).toBe(true)
  })

  it('换一个地址后重新尝试加载（不能一直停在回退态）', async () => {
    const wrapper = mount(AdminAvatar, { props: { src: 'https://cdn.test/gone.png', name: '张三' } })

    await wrapper.find('[data-role="admin-avatar-image"] img').trigger('error')
    expect(wrapper.find('[data-role="admin-avatar-initial"]').exists()).toBe(true)

    await wrapper.setProps({ src: 'https://cdn.test/new.png' })

    expect(wrapper.find('[data-role="admin-avatar-image"]').exists()).toBe(true)
    expect(wrapper.find('[data-role="admin-avatar-initial"]').exists()).toBe(false)
  })
})
