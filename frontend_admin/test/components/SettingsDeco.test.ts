import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp, nextTick, ref as vueRef, onMounted as vueOnMounted } from "vue";

/**
 * 装饰表单的「滑块 + 数字」双输入（组件层用例）。
 *
 * 为什么用桩组件而不是真挂 naive-ui：后台没有组件测试基线，真组件需要一整套
 * 主题、浮层与交互环境；这里只验证**接线**：同一份 `config.charm[key]` 同时喂给
 * NSlider 与 NInputNumber，任一端的 update 都写回同一个字段，另一端立刻看到新值；
 * 数字输入越界时按元数据的钳位规则夹回并在行内给出可见提示。
 *
 * 桩把 Naive 的两个输入渲染成原生 input：[data-role="NSlider"] /
 * [data-role="NInputNumber"]，输入事件转发成 `update:value`，与真组件同形。
 */

const { messageMock } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("~/services/admin", () => ({
  getDecoRuntimeConfig: vi.fn(async () => ({ charm: { enable: true } })),
  updateDecoRuntimeConfig: vi.fn(async () => undefined),
}));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  function inputStub(name: string) {
    return defineComponent({
      name,
      inheritAttrs: false,
      props: {
        value: { default: undefined },
        min: { type: Number, default: undefined },
        max: { type: Number, default: undefined },
        step: { type: Number, default: undefined },
        precision: { type: Number, default: undefined },
        disabled: { type: Boolean, default: false },
      },
      emits: ["update:value"],
      setup(props, { emit, slots, attrs }) {
        return () =>
          h("div", { ...attrs, "data-stub": name }, [
            h("input", {
              "data-role": name,
              type: "number",
              value: props.value ?? "",
              min: props.min,
              max: props.max,
              step: props.step,
              disabled: props.disabled,
              onInput: (event: Event) => {
                const raw = (event.target as HTMLInputElement).value;
                emit("update:value", raw === "" ? null : Number(raw));
              },
            }),
            slots.suffix ? slots.suffix() : null,
          ]);
      },
    });
  }

  const passthrough = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: false,
      setup(_, { slots, attrs }) {
        return () => h("div", { ...attrs, "data-stub": name }, slots.default ? slots.default() : []);
      },
    });

  const formItemStub = defineComponent({
    name: "NFormItem",
    inheritAttrs: false,
    props: { label: { type: String, default: "" }, feedback: { type: String, default: "" } },
    setup(props, { slots, attrs }) {
      return () =>
        h("div", { ...attrs, "data-stub": "NFormItem", "data-label": props.label }, [
          h("span", { "data-role": "label" }, props.label),
          slots.default ? slots.default() : null,
          h("span", { "data-role": "feedback" }, props.feedback),
        ]);
    },
  });

    const switchStub = defineComponent({
      name: "NSwitch",
      inheritAttrs: false,
      props: { value: { type: Boolean, default: false } },
      setup(props, { attrs }) {
        return () => h("div", { ...attrs, "data-stub": "NSwitch", "data-value": String(props.value) });
      },
    });

  return {
    NAlert: passthrough("NAlert"),
    NButton: passthrough("NButton"),
    NForm: passthrough("NForm"),
    NFormItem: formItemStub,
    NInputNumber: inputStub("NInputNumber"),
    NSlider: inputStub("NSlider"),
    NSwitch: switchStub,
    useMessage: () => messageMock,
  };
});

// Nuxt 自动导入（script setup 顶层就用到 ref / onMounted）：测试里先挂到全局，
// 再动态 import 组件，避免求值顺序问题。不要改成静态 import。
Object.assign(globalThis, { ref: vueRef, onMounted: vueOnMounted });

const { default: SettingsDeco } = await import("~/components/settings/SettingsDeco.vue");
const { DECO_NUMBER_PARAMS } = await import("~/utils/decoParams");

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = [];

async function mountDeco() {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const app = createApp(SettingsDeco);
  app.mount(host);
  mounted.push({ app, host });
  await vi.waitFor(() => {
    expect(host.querySelector('[data-stub="NSlider"]')).not.toBeNull();
  });
  return host;
}

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount();
    host.remove();
  }
});

function field(host: HTMLElement, key: string, stub: "NSlider" | "NInputNumber"): HTMLInputElement {
  const input = host.querySelector(
    `[data-stub="${stub}"][data-param="${key}"] input[data-role="${stub}"]`,
  );
  expect(input, `${stub} 里没有 data-param="${key}" 的输入`).not.toBeNull();
  return input as HTMLInputElement;
}

async function type(host: HTMLElement, stub: "NSlider" | "NInputNumber", key: string, value: string) {
  const input = field(host, key, stub);
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await nextTick();
  await nextTick();
}

function notice(host: HTMLElement): string | null {
  return host.querySelector("[data-clamp-notice]")?.textContent?.trim() ?? null;
}

describe("装饰表单：每个数值参数都是滑块 + 数字双输入", () => {
  it("十二个参数各渲染一个滑块与一个数字输入，且两者共用同一份元数据区间", async () => {
    const host = await mountDeco();

    expect(host.querySelectorAll('[data-stub="NSlider"]')).toHaveLength(DECO_NUMBER_PARAMS.length);
    expect(host.querySelectorAll('[data-stub="NInputNumber"]')).toHaveLength(DECO_NUMBER_PARAMS.length);

    for (const param of DECO_NUMBER_PARAMS) {
      const slider = field(host, param.key, "NSlider");
      const number = field(host, param.key, "NInputNumber");
      expect(slider.getAttribute("min"), `${param.key} 滑块下限`).toBe(String(param.min));
      expect(slider.getAttribute("max"), `${param.key} 滑块上限`).toBe(String(param.max));
      expect(slider.getAttribute("step"), `${param.key} 滑块步长`).toBe(String(param.step));
      expect(number.getAttribute("min"), `${param.key} 数字下限`).toBe(String(param.min));
      expect(number.getAttribute("max"), `${param.key} 数字上限`).toBe(String(param.max));
    }
  });

  it("滑块改动立刻同步到数字框", async () => {
    const host = await mountDeco();

    await type(host, "NSlider", "segments", "16");
    expect(field(host, "segments", "NInputNumber").value).toBe("16");
  });

  it("数字输入改动立刻同步到滑块", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "damping", "1");
    expect(field(host, "damping", "NSlider").value).toBe("1");
  });
});

describe("装饰表单：越界与非法输入按钳位规则夹回，并给出可见反馈", () => {
  it("数字框填 100 节 → 夹回 30、滑块同步、行内出现提示", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "segments", "100");

    expect(field(host, "segments", "NInputNumber").value).toBe("30");
    expect(field(host, "segments", "NSlider").value).toBe("30");
    expect(notice(host)).toContain("4–30 节");
    expect(notice(host)).toContain("100 → 30");
  });

  it("数字框填 10 px（站标尺寸下限 15）→ 夹回 15 并提示", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "logo_size", "10");

    expect(field(host, "logo_size", "NInputNumber").value).toBe("15");
    expect(notice(host)).toContain("15–70 px");
  });

  it("数字框清空 → 恢复默认 48，不崩且有提示", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "logo_size", "");

    expect(field(host, "logo_size", "NInputNumber").value).toBe("48");
    expect(notice(host)).toContain("已恢复默认 48");
  });

  it("数字框填负数 → 恢复默认而不是夹到下限", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "logo_size", "-5");

    expect(field(host, "logo_size", "NInputNumber").value).toBe("48");
    expect(notice(host)).toContain("-5");
  });

  it("数字框填超大值 → 夹到上限，继续输入合法值时提示消失", async () => {
    const host = await mountDeco();

    await type(host, "NInputNumber", "segments", "1000000000");
    expect(field(host, "segments", "NInputNumber").value).toBe("30");
    expect(notice(host)).not.toBeNull();

    await type(host, "NInputNumber", "segments", "12");
    expect(field(host, "segments", "NInputNumber").value).toBe("12");
    expect(field(host, "segments", "NSlider").value).toBe("12");
    expect(notice(host)).toBeNull();
  });
});
