import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadImageByTask } from '~/services/image'

const requestMock = vi.fn()

/**
 * 后台 services/image.ts 的用途传参（#216）：与读者站那份同源形态，
 * 创建上传任务的请求体必须带 purpose；`purpose: null` 则完全不带该字段。
 * 后台没有 #imports 别名，这里直接把 Nuxt 自动导入的 useNuxtApp 塞进全局。
 */
describe('admin image service purpose', () => {
  beforeEach(() => {
    vi.stubGlobal('useNuxtApp', () => ({ $api: { request: requestMock } }))
    requestMock.mockReset()
  })

  async function skipUpload() {
    requestMock.mockResolvedValue({
      skip_upload: true,
      url: 'https://image.example.com/cover.png',
      hash: 'etag-1',
      image_id: 'img-1',
      status: 'ready',
    })
    return uploadImageByTask(new File(['cover'], 'cover.png', { type: 'image/png' }), { purpose: 'cover' })
  }

  it('declares cover purpose on the create-task request', async () => {
    await skipUpload()

    expect(requestMock).toHaveBeenCalledWith('/images/upload-tasks', expect.objectContaining({
      method: 'POST',
      body: expect.objectContaining({ purpose: 'cover' }),
    }))
  })

  it('defaults to cover when purpose is omitted', async () => {
    requestMock.mockResolvedValue({ skip_upload: true, url: 'https://image.example.com/cover.png' })

    await uploadImageByTask(new File(['cover'], 'cover.png', { type: 'image/png' }))

    expect(requestMock.mock.calls[0]?.[1]?.body).toEqual(expect.objectContaining({ purpose: 'cover' }))
  })

  it('omits purpose entirely when the caller declares none', async () => {
    requestMock.mockResolvedValue({ skip_upload: true, url: 'https://image.example.com/photo.png' })

    await uploadImageByTask(
      new File(['photo'], 'photo.png', { type: 'image/png' }),
      { purpose: null },
    )

    expect(requestMock.mock.calls[0]?.[1]?.body).not.toHaveProperty('purpose')
  })
})
