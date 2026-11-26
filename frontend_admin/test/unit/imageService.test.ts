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
      audit_status: 'pass',
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
 /**
  * 秒传命中"已不可用"的图（ADR-0034：未过审不给地址）时的提示。
  *
  * 后台这份客户端以前只看 `audit_status`，而秒传响应里根本没这个字段
  * （只有任务状态 `status: ready`），于是封禁 / 已删除的图都报成
  * "上传任务命中秒传，但未返回图片 URL"，看起来像上传坏了。
  */
 describe('admin image service audit outcome', () => {
   beforeEach(() => {
     vi.stubGlobal('useNuxtApp', () => ({ $api: { request: requestMock } }))
     requestMock.mockReset()
   })

   it.each([
     ['block', '图片包含违规内容，已被拒绝并删除，请换一张'],
     ['deleted', '这张图已被删除，请换一张'],
     ['orphaned', '这张图已失效，请换一张'],
     ['unknown', '这张图状态异常，请换一张后重试'],
   ])('秒传命中 audit_status=%s 时给出可操作提示', async (auditStatus, expected) => {
     requestMock.mockResolvedValue({
       skip_upload: true,
       url: '',
       hash: 'etag-2',
       image_id: 'img-2',
       status: 'ready',
       audit_status: auditStatus,
     })

     await expect(
       uploadImageByTask(new File(['cover'], 'cover.png', { type: 'image/png' }), { purpose: 'cover' }),
     ).rejects.toThrow(expected)
   })

   it.each([
     ['pending', '这张图之前已上传，正在机器审核（通常几秒，通过后自动生效）'],
     ['review', '这张图之前已上传，正在人工审核（通过后才会生效）'],
   ])('秒传命中 audit_status=%s 时报「审核中」', async (auditStatus, expected) => {
     requestMock.mockResolvedValue({
       skip_upload: true,
       url: '',
       hash: 'etag-2',
       image_id: 'img-2',
       status: 'ready',
       audit_status: auditStatus,
     })

     await expect(
       uploadImageByTask(new File(['cover'], 'cover.png', { type: 'image/png' }), { purpose: 'cover' }),
     ).rejects.toThrow(expected)
   })

   it('秒传命中审核通过却仍无地址时按真异常报出', async () => {
     requestMock.mockResolvedValue({
       skip_upload: true,
       url: '',
       hash: 'etag-2',
       image_id: 'img-2',
       status: 'ready',
       audit_status: 'pass',
     })

     await expect(
       uploadImageByTask(new File(['cover'], 'cover.png', { type: 'image/png' }), { purpose: 'cover' }),
     ).rejects.toThrow('上传任务命中秒传，但未返回图片 URL')
   })
 })
