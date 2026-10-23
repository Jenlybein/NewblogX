import type {
  CreateImageUploadTaskRequest,
  CreateImageUploadTaskResponseData,
  UploadTaskStatusResponseData,
} from '~/types/api'

const QINIU_UPLOAD_HOST_BY_REGION: Record<string, string> = {
  z0: 'https://upload.qiniup.com',
  z1: 'https://upload-z1.qiniup.com',
  z2: 'https://upload-z2.qiniup.com',
  na0: 'https://upload-na0.qiniup.com',
  as0: 'https://upload-as0.qiniup.com',
  ap: 'https://upload-ap-southeast-1.qiniup.com',
}
const DEFAULT_QINIU_UPLOAD_HOST = QINIU_UPLOAD_HOST_BY_REGION.z0 ?? 'https://upload.qiniup.com'

type UploadStage =
  | 'hashing'
  | 'creating_task'
  | 'uploading_to_qiniu'
  | 'polling_status'

export interface UploadImageByTaskOptions {
  pollIntervalMs?: number
  pollTimeoutMs?: number
  onStage?: (stage: UploadStage) => void
  signal?: AbortSignal
}

export interface UploadImageByTaskResult {
  url: string
  hash?: string
  image_id?: string
  upload_id?: string
  status?: string
  skip_upload: boolean
}

function api() {
  return useNuxtApp().$api
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms))
}

function base64UrlFromBytes(bytes: Uint8Array) {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

function rotateLeft(value: number, bits: number) {
  return ((value << bits) | (value >>> (32 - bits))) >>> 0
}

function sha1BytesFallback(buffer: ArrayBuffer) {
  const input = new Uint8Array(buffer)
  const bitLength = input.length * 8
  const totalLength = Math.ceil((input.length + 9) / 64) * 64
  const message = new Uint8Array(totalLength)
  message.set(input)
  message[input.length] = 0x80

  const view = new DataView(message.buffer)
  view.setUint32(totalLength - 8, Math.floor(bitLength / 0x100000000), false)
  view.setUint32(totalLength - 4, bitLength >>> 0, false)

  let h0 = 0x67452301
  let h1 = 0xefcdab89
  let h2 = 0x98badcfe
  let h3 = 0x10325476
  let h4 = 0xc3d2e1f0
  const words = new Uint32Array(80)

  for (let offset = 0; offset < totalLength; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getUint32(offset + index * 4, false)
    }

    for (let index = 16; index < 80; index += 1) {
      words[index] = rotateLeft(words[index - 3]! ^ words[index - 8]! ^ words[index - 14]! ^ words[index - 16]!, 1)
    }

    let a = h0
    let b = h1
    let c = h2
    let d = h3
    let e = h4

    for (let index = 0; index < 80; index += 1) {
      let f = 0
      let k = 0

      if (index < 20) {
        f = (b & c) | (~b & d)
        k = 0x5a827999
      } else if (index < 40) {
        f = b ^ c ^ d
        k = 0x6ed9eba1
      } else if (index < 60) {
        f = (b & c) | (b & d) | (c & d)
        k = 0x8f1bbcdc
      } else {
        f = b ^ c ^ d
        k = 0xca62c1d6
      }

      const temp = (rotateLeft(a, 5) + f + e + k + words[index]!) >>> 0
      e = d
      d = c
      c = rotateLeft(b, 30)
      b = a
      a = temp
    }

    h0 = (h0 + a) >>> 0
    h1 = (h1 + b) >>> 0
    h2 = (h2 + c) >>> 0
    h3 = (h3 + d) >>> 0
    h4 = (h4 + e) >>> 0
  }

  const digest = new Uint8Array(20)
  const digestView = new DataView(digest.buffer)
  const digestWords = [h0, h1, h2, h3, h4]
  digestWords.forEach((value, index) => digestView.setUint32(index * 4, value, false))
  return digest
}

async function sha1Bytes(buffer: ArrayBuffer) {
  const subtle = globalThis.crypto?.subtle
  if (subtle?.digest) {
    const digest = await subtle.digest('SHA-1', buffer)
    return new Uint8Array(digest)
  }

  return sha1BytesFallback(buffer)
}

export async function computeQetag(file: File) {
  const blockSize = 4 * 1024 * 1024

  if (file.size <= blockSize) {
    const buffer = await file.arrayBuffer()
    const sha1 = await sha1Bytes(buffer)
    const result = new Uint8Array(1 + sha1.length)
    result[0] = 0x16
    result.set(sha1, 1)
    return base64UrlFromBytes(result)
  }

  const parts: Uint8Array[] = []
  for (let start = 0; start < file.size; start += blockSize) {
    const end = Math.min(start + blockSize, file.size)
    const chunk = await file.slice(start, end).arrayBuffer()
    const sha1 = await sha1Bytes(chunk)
    parts.push(sha1)
  }

  const merged = new Uint8Array(parts.length * 20)
  parts.forEach((part, index) => merged.set(part, index * 20))
  const allSha1 = await sha1Bytes(merged.buffer)
  const result = new Uint8Array(1 + allSha1.length)
  result[0] = 0x96
  result.set(allSha1, 1)
  return base64UrlFromBytes(result)
}

export function createImageUploadTask(payload: CreateImageUploadTaskRequest) {
  return api().request<CreateImageUploadTaskResponseData, CreateImageUploadTaskRequest>('/images/upload-tasks', {
    method: 'POST',
    body: payload,
  })
}

export function getImageUploadTaskStatus(uploadId: string) {
  return api().request<UploadTaskStatusResponseData>(`/images/upload-tasks/${uploadId}`)
}

function getQiniuUploadHost(region?: string) {
  if (!region) return DEFAULT_QINIU_UPLOAD_HOST
  return QINIU_UPLOAD_HOST_BY_REGION[region] ?? DEFAULT_QINIU_UPLOAD_HOST
}

async function uploadToQiniu(params: {
  file: File
  uploadToken: string
  objectKey: string
  region?: string
  signal?: AbortSignal
}) {
  const formData = new FormData()
  formData.append('token', params.uploadToken)
  formData.append('key', params.objectKey)
  formData.append('file', params.file, params.file.name)

  const response = await fetch(getQiniuUploadHost(params.region), {
    method: 'POST',
    body: formData,
    signal: params.signal,
  })

  if (!response.ok) {
    throw new Error(`上传七牛失败（HTTP ${response.status}）`)
  }
}

async function waitUploadReady(
  uploadId: string,
  options: Pick<UploadImageByTaskOptions, 'pollIntervalMs' | 'pollTimeoutMs' | 'signal'>,
) {
  const pollIntervalMs = options.pollIntervalMs ?? 1500
  const pollTimeoutMs = options.pollTimeoutMs ?? 30000
  const startedAt = Date.now()

  while (Date.now() - startedAt < pollTimeoutMs) {
    if (options.signal?.aborted) {
      throw new Error('上传已取消')
    }

    const status = await getImageUploadTaskStatus(uploadId)
    if (status.status === 'ready') {
      return status
    }

    if (status.status === 'failed') {
      throw new Error(status.error_msg || '图片上传任务失败')
    }

    await sleep(pollIntervalMs)
  }

  throw new Error('图片上传确认超时，请稍后重试')
}

export async function uploadImageByTask(file: File, options: UploadImageByTaskOptions = {}): Promise<UploadImageByTaskResult> {
  options.onStage?.('hashing')
  const hash = await computeQetag(file)

  options.onStage?.('creating_task')
  const task = await createImageUploadTask({
    file_name: file.name,
    size: file.size,
    mime_type: file.type || 'application/octet-stream',
    hash,
  })

  if (task.skip_upload) {
    if (!task.url) {
      throw new Error('上传任务命中秒传，但未返回图片 URL')
    }

    return {
      url: task.url,
      hash: task.hash,
      image_id: task.image_id,
      status: task.status,
      skip_upload: true,
    }
  }

  if (!task.upload_id || !task.upload_token || !task.object_key) {
    throw new Error('上传任务缺少必要字段（upload_id/upload_token/object_key）')
  }

  options.onStage?.('uploading_to_qiniu')
  await uploadToQiniu({
    file,
    uploadToken: task.upload_token,
    objectKey: task.object_key,
    region: task.region,
    signal: options.signal,
  })

  options.onStage?.('polling_status')
  const status = await waitUploadReady(task.upload_id, options)
  if (!status.url) {
    throw new Error('上传任务已就绪，但未返回图片 URL')
  }

  return {
    url: status.url,
    hash: status.hash,
    image_id: status.image_id,
    upload_id: status.upload_id,
    status: status.status,
    skip_upload: false,
  }
}
