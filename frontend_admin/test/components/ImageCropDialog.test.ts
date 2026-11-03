import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ImageCropDialog from '~/components/media/ImageCropDialog.vue'
import type { ShapeFramingState } from '~/utils/imageShaping'

/**
 * 后台裁剪弹层外壳（票 242）：以前 `NModal` 只传 `preset="card"`，没有 width——
 * naive-ui 的 `NCard` 自带 `width: 100%`、`.n-modal` 没有宽度，于是弹层被最长
 * 中文提示行撑到接近窗口宽；再加上取景框按宽度派生高度，观感就是"铺满窗口"。
 *
 * 这里断言的是**外壳交给 naive-ui 什么尺寸配置**，以及三个出口：确认（带上
 * 取景状态）、取消、弹层自己发起的关闭（右上角 X / Esc）也算取消。
 * NModal 被替换成只记录 prop 的壳：naive-ui 自己的渲染不是这一层的职责。
 * 两端这份组件逐字节同源（`node scripts/check-image-shaping-parity.mjs`）。
 */
const rendered = vi.hoisted(() => [] as Array<Record<string, unknown>>)

vi.mock('naive-ui', () => {
  const NModal = defineComponent({
    name: 'NModal',
    props: {
      show: { type: Boolean, default: false },
      title: { type: String, default: '' },
      bordered: { type: Boolean, default: true },
      maskClosable: { type: Boolean, default: true },
      style: { type: [String, Object], default: undefined },
    },
    emits: ['update:show'],
    setup(props, { slots, emit }) {
      return () => {
        if (!props.show) return null
        rendered.push({ ...props })
        return h('div', { 'data-role': 'n-modal', 'data-style': JSON.stringify(props.style ?? null) }, [
          slots.default?.(),
          // 代表右上角 X / Esc：弹层自己发起的关闭。
          h('button', { 'data-role': 'n-modal-close', onClick: () => emit('update:show', false) }, '×'),
        ])
      }
    },
  })

  return { NModal }
})

interface Recorded {
  confirm: ShapeFramingState[]
  cancel: number
  show: boolean[]
}

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = []

/** 后台没有 @vue/test-utils：用原生 createApp + DOM 事件驱动（与线上事件流一致）。 */
function mountDialog(overrides: Record<string, unknown> = {}) {
  const events: Recorded = { confirm: [], cancel: 0, show: [] }
  const Wrapper = defineComponent({
    name: 'CropDialogHarness',
    setup() {
      return () =>
        h(ImageCropDialog, {
          show: true,
          src: 'blob:original',
          imageWidth: 2000,
          imageHeight: 900,
          target: { width: 1600, height: 900 },
          ratioText: '16:9',
          title: '裁剪封面',
          ...overrides,
          onConfirm: (framing: ShapeFramingState) => events.confirm.push(framing),
          onCancel: () => {
            events.cancel += 1
          },
          'onUpdate:show': (value: boolean) => events.show.push(value),
        })
    },
  })

  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(Wrapper)
  app.mount(host)
  mounted.push({ app, host })

  const modalStyle = () => JSON.parse(host.querySelector('[data-role="n-modal"]')?.getAttribute('data-style') ?? '{}')
  const button = (label: string) => host.querySelector(`button[aria-label="${label}"]`) as HTMLButtonElement | null
  const closeButton = () => host.querySelector('[data-role="n-modal-close"]') as HTMLButtonElement | null

  return { host, events, modalStyle, button, closeButton }
}

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount()
    host.remove()
  }
})

describe('后台裁剪弹层：尺寸（票 242）', () => {
  it('宽度封顶 560px；92vw 在移动端恰好留出左右各 4vw 边距', () => {
    const { modalStyle } = mountDialog()
    const style = modalStyle() as Record<string, string>

    expect(style.width).toBe('min(92vw, 560px)')
    // 92vw + 2 × 4vw = 100vw：窄屏不再贴边。
    expect(style.width).toContain('92vw')
  })

  it('高度不超过视口：max-height 是 100dvh 减去上下 24px 留白', () => {
    const { modalStyle } = mountDialog()
    const style = modalStyle() as Record<string, string>

    expect(style.maxHeight).toBe('calc(100dvh - 48px)')
  })

  it('遮罩误触仍然关不掉（尺寸变化不放松这条约束）', () => {
    mountDialog()
    const props = rendered.at(-1) ?? {}

    expect(props.maskClosable).toBe(false)
    expect(props.bordered).toBe(false)
  })
})

describe('后台裁剪弹层：三个出口（确认 / 取消 / 弹层自关）', () => {
  it('点确认：上抛取景状态并关窗', async () => {
    const { events, button } = mountDialog()
    button('确认裁剪')?.click()
    await nextTick()

    expect(events.confirm.at(-1)).toEqual({ zoom: 1, centerX: 0.5, centerY: 0.5 })
    expect(events.show.at(-1)).toBe(false)
  })

  it('点取消：只抛 cancel，不上传任何取景状态', async () => {
    const { events, button } = mountDialog()
    button('取消裁剪')?.click()
    await nextTick()

    expect(events.cancel).toBe(1)
    expect(events.confirm).toHaveLength(0)
  })

  it('弹层自己关（X / Esc / 遮罩）也算取消；确认之后的关窗不再重复结算', async () => {
    const first = mountDialog()
    first.closeButton()?.click()
    await nextTick()
    expect(first.events.cancel).toBe(1)
    expect(first.events.confirm).toHaveLength(0)

    const second = mountDialog()
    second.button('确认裁剪')?.click()
    await nextTick()
    second.closeButton()?.click()
    await nextTick()
    // 确认后弹层的 update:show(false) 与确认是同一次开关的收尾：不能再抛 cancel。
    expect(second.events.cancel).toBe(0)
    expect(second.events.confirm).toHaveLength(1)
  })
})
