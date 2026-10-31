import { createApp, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import BannerFramingView from "~/components/media/BannerFramingView.vue";
import type { Framing } from "~/utils/framing";

/**
 * 取景框组件的三条交互（#45 的验收口径）：
 *   1. 拖动产出取景参数；
 *   2. 缩放产出取景参数；
 *   3. 取消不上抛任何取景参数。
 * 外加键盘平移/缩放（验收里"键盘可操作"那一条）。
 *
 * 断言只看**对外可观察的东西**：组件抛出的参数、图片上最终生效的那三条
 * CSS 声明。不去碰内部 ref 名、也不断言 class 的具体拼写（选择器用语义选择器
 * 与 aria-label，改了内部结构仍然成立）。
 *
 * 不用 @vue/test-utils：后台这个应用没装它，而前端依赖是离线安装的，
 * 为一条断言去改 lockfile 不划算。createApp + 原生事件已经足够表达"用户做了什么"。
 */

interface ViewEvents {
  confirmed?: Framing;
  cancelled: number;
  updates: Framing[];
}

let app: ReturnType<typeof createApp> | null = null;
let host: HTMLDivElement | null = null;

function mountView(props: Record<string, unknown>) {
  const events: ViewEvents = { cancelled: 0, updates: [] };
  const container = document.createElement("div");
  document.body.appendChild(container);

  app = createApp({
    render: () =>
      h(BannerFramingView, {
        src: "/demo.jpg",
        imageWidth: 3200,
        imageHeight: 900,
        ...props,
        onConfirm: (framing: Framing) => {
          events.confirmed = framing;
        },
        onCancel: () => {
          events.cancelled += 1;
        },
        "onUpdate:framing": (framing: Framing) => {
          events.updates.push(framing);
        },
      }),
  });
  app.mount(container);
  host = container;

  const frame = container.querySelector<HTMLElement>(".admin-framing__frame");
  const image = container.querySelector<HTMLImageElement>(".admin-framing__image");
  if (!frame || !image) throw new Error("取景框没有渲染出来");

  return { container, frame, image, events };
}

function pointer(type: string, clientX: number, clientY: number) {
  const event = new MouseEvent(type, { bubbles: true, clientX, clientY });
  return event;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

afterEach(() => {
  app?.unmount();
  app = null;
  host?.remove();
  host = null;
});

describe("BannerFramingView", () => {
  it("拖动产出取景参数：拖到角落就停在角落，图片仍然铺满取景框", async () => {
    const { frame, image, events } = mountView({});

    frame.dispatchEvent(pointer("pointerdown", 400, 200));
    frame.dispatchEvent(pointer("pointermove", 480, 200));
    frame.dispatchEvent(pointer("pointerup", 480, 200));

    // 3200×900 的宽幅图：往右拖 80px，焦点 x 从 0.5 减 0.1。
    expect(events.updates.at(-1)).toEqual({ focus_x: 0.4, focus_y: 0.5, zoom: 1 });

    // 一路拖到底：焦点贴边 0/1，不会变成负数，也不会让 object-position 出界（露白）。
    frame.dispatchEvent(pointer("pointerdown", 400, 200));
    frame.dispatchEvent(pointer("pointermove", 99999, 200));
    frame.dispatchEvent(pointer("pointerup", 99999, 200));
    const last = events.updates.at(-1)!;
    expect(last.focus_x).toBe(0);

    await nextTick();
    expect(image.style.objectPosition).toBe("0% 50%");
  });

  it("缩放产出取景参数：滑杆与加减号都按 0.05 步进，且钳在 1–2", async () => {
    const { image, events, container } = mountView({});

    const slider = container.querySelector<HTMLInputElement>('input[type="range"]');
    if (!slider) throw new Error("没有缩放滑杆");
    slider.value = "1.5";
    slider.dispatchEvent(new Event("input", { bubbles: true }));
    expect(events.updates.at(-1)).toEqual({ focus_x: 0.5, focus_y: 0.5, zoom: 1.5 });

    await nextTick();
    expect(image.style.transform).toBe("scale(1.5)");

    const zoomIn = container.querySelector<HTMLButtonElement>('[aria-label="放大"]');
    const zoomOut = container.querySelector<HTMLButtonElement>('[aria-label="缩小"]');
    if (!zoomIn || !zoomOut) throw new Error("没有加减号按钮");

    zoomIn.click();
    expect(events.updates.at(-1)!.zoom).toBe(1.55);
    for (let i = 0; i < 20; i += 1) zoomIn.click();
    expect(events.updates.at(-1)!.zoom).toBe(2);
    for (let i = 0; i < 40; i += 1) zoomOut.click();
    expect(events.updates.at(-1)!.zoom).toBe(1);
  });

  it("取消不上抛任何取景参数（也不抛确认）", () => {
    const { container, events } = mountView({});

    const cancel = container.querySelector<HTMLButtonElement>('[aria-label="取消取景"]');
    if (!cancel) throw new Error("没有取消按钮");
    cancel.click();

    expect(events.cancelled).toBe(1);
    expect(events.confirmed).toBeUndefined();
    expect(events.updates).toHaveLength(0);
  });

  it("确认上抛 {focus_x, focus_y, zoom}（缺省即居中、zoom 1）", () => {
    const { container, events } = mountView({});

    const confirm = container.querySelector<HTMLButtonElement>('[aria-label="确认取景"]');
    if (!confirm) throw new Error("没有确认按钮");
    confirm.click();

    expect(events.confirmed).toEqual({ focus_x: 0.5, focus_y: 0.5, zoom: 1 });
    expect(events.cancelled).toBe(0);
  });

  it("键盘可操作：方向键移动取景范围、加减号缩放", () => {
    const { frame, events } = mountView({});

    frame.focus();
    frame.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    frame.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true }));
    expect(events.updates.at(-1)).toEqual({ focus_x: 0.52, focus_y: 0.48, zoom: 1 });

    frame.dispatchEvent(new KeyboardEvent("keydown", { key: "+", bubbles: true }));
    expect(events.updates.at(-1)!.zoom).toBe(1.05);
    frame.dispatchEvent(new KeyboardEvent("keydown", { key: "-", bubbles: true }));
    expect(events.updates.at(-1)!.zoom).toBe(1);
  });

  it("传入已有的取景参数时按它开场（重新取景不改动旧值）", () => {
    const { container, events } = mountView({ framing: { focus_x: 0.2, focus_y: 0.8, zoom: 1.5 } });

    const confirm = container.querySelector<HTMLButtonElement>('[aria-label="确认取景"]');
    confirm?.click();

    expect(events.confirmed).toEqual({ focus_x: 0.2, focus_y: 0.8, zoom: 1.5 });
  });
});
