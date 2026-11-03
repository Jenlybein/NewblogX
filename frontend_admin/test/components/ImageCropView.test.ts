import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import ImageCropView from '~/components/media/ImageCropView.vue'
import type { ShapeFramingState } from '~/utils/imageShaping'

/**
 * 后台裁剪窗的交互面（#215）。
 *
 * 后台没有 `@vue/test-utils`（必须自洽、不引入读者站测试依赖），所以用原生的
 * createApp + DOM 事件驱动——这也更接近线上：测的是真实事件流，而不是包装层。
 * 比例框固定、拖动画面、方向键/缩放钳位、Esc 取消与确认两个出口。
 */

interface Recorded {
  confirm: ShapeFramingState[]
  cancel: number
  framing: ShapeFramingState[]
}

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = []

function mountView(props: Record<string, unknown> = {}) {
  const events: Recorded = { confirm: [], cancel: 0, framing: [] }
  const Wrapper = defineComponent({
    name: 'CropViewHarness',
    setup() {
      return () =>
        h(ImageCropView, {
          src: 'blob:original',
          imageWidth: 2000,
          imageHeight: 900,
          target: { width: 1600, height: 900 },
          ratioText: '16:9',
          ...props,
          onConfirm: (framing: ShapeFramingState) => events.confirm.push(framing),
          onCancel: () => {
            events.cancel += 1
          },
          'onUpdate:framing': (framing: ShapeFramingState) => events.framing.push(framing),
        })
    },
  })

  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(Wrapper)
  app.mount(host)
  mounted.push({ app, host })

  const frame = () => host.querySelector('[data-role="crop-frame"]') as HTMLElement
  const button = (label: string) =>
    host.querySelector(`button[aria-label="${label}"]`) as HTMLButtonElement | null

  return { host, events, frame, button }
}

function frameRatio(frame: HTMLElement): number {
  const style = frame.getAttribute('style') ?? ''
  const value = (/aspect-ratio:\s*([^;]+);/u.exec(style)?.[1] ?? '').trim()
  const [width, height] = value.split('/').map(part => Number(part.trim()))
  expect(Number.isFinite(width) && Number.isFinite(height) && height > 0).toBe(true)
  return width / height
}

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount()
    host.remove()
  }
})

describe('后台裁剪窗：固定比例框与键盘可达', () => {
  it('框比例来自目标（1600×900 = 16:9），框可聚焦且有操作说明', () => {
    const { frame } = mountView()
    const el = frame()

    expect(frameRatio(el)).toBeCloseTo(16 / 9, 6)
    expect(el.getAttribute('tabindex')).toBe('0')
    expect(el.getAttribute('role')).toBe('group')
    expect(el.getAttribute('aria-label')).toContain('16:9')
    expect(el.getAttribute('aria-label')).toContain('方向键')
  })

  it('预览画面由源矩形反推：与最终烤进文件的是同一组数', () => {
    const { host } = mountView()
    const image = host.querySelector('[data-role="crop-image"]') as HTMLElement

    // 2000×900 取 16:9：可视区 1600×900、左边裁掉 200 → 图宽 125%、左移 12.5%。
    expect(image.style.left).toBe('-12.5%')
    expect(image.style.width).toBe('125%')
    expect(image.style.height).toBe('100%')
  })
})

describe('后台裁剪窗：拖动、方向键与缩放', () => {
  it('拖动画面换算成取景状态，确认时带上它', async () => {
    const { events, frame, button } = mountView()
    const el = frame()

    el.dispatchEvent(new MouseEvent('pointerdown', { clientX: 100, clientY: 50, button: 0, bubbles: true }))
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 180, clientY: 50 }))
    await nextTick()
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 180, clientY: 50 }))
    await nextTick()

    button('确认裁剪')?.click()
    await nextTick()

    // 拖了 80/800 = 10% 视口宽：取景中心左移 0.08。
    expect(events.confirm.at(-1)?.centerX).toBeCloseTo(0.42, 6)
  })

  it('方向键右移取景范围；加减号缩放钳在 1–3×', async () => {
    const { events, frame, host, button } = mountView()
    const el = frame()

    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    await nextTick()
    expect(events.framing.at(-1)?.centerX).toBeCloseTo(0.54, 6)

    const slider = host.querySelector('input[aria-label="缩放"]') as HTMLInputElement
    slider.value = '5'
    slider.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(events.framing.at(-1)?.zoom).toBe(3)
    expect(button('放大')?.disabled).toBe(true)

    el.dispatchEvent(new KeyboardEvent('keydown', { key: '-', bubbles: true }))
    await nextTick()
    expect(events.framing.at(-1)?.zoom).toBeCloseTo(2.9, 6)
  })
})

describe('后台裁剪窗：确认与取消是两个出口', () => {
  it('Esc 取消，不上抛任何取景状态', async () => {
    const { events, frame } = mountView()
    frame().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(events.cancel).toBe(1)
    expect(events.confirm).toHaveLength(0)
  })

  it('取消按钮只抛 cancel；确认按钮抛钳位后的取景状态', async () => {
    const first = mountView()
    first.button('取消裁剪')?.click()
    await nextTick()
    expect(first.events.cancel).toBe(1)
    expect(first.events.confirm).toHaveLength(0)

    const second = mountView()
    const slider = second.host.querySelector('input[aria-label="缩放"]') as HTMLInputElement
    slider.value = '0.4'
    slider.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    second.button('确认裁剪')?.click()
    await nextTick()

    expect(second.events.confirm.at(-1)).toEqual({ zoom: 1, centerX: 0.5, centerY: 0.5 })
  })

  it('没有旋转 / 镜像 / 滤镜入口：只有拖动、缩放、重置、确认、取消', () => {
    const { host } = mountView()
    const labels = Array.from(host.querySelectorAll('button')).map(button => button.getAttribute('aria-label'))

    expect(labels).toEqual(['缩小', '放大', '重置取景', '取消裁剪', '确认裁剪'])
    expect(host.innerHTML).not.toMatch(/rotate|scaleX|mirror|filter:/u)
  })
})
