import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { reactive, type Ref } from "vue";

import CursorCharm from "~/components/deco/CursorCharm.vue";
import {
  CHARM_DAMPING_DEFAULT,
  CHARM_FPS_CAP_DEFAULT,
  CHARM_GRAVITY_DEFAULT,
  CHARM_IDLE_SWAY_AMP_DEFAULT,
  CHARM_IDLE_SWAY_HZ_DEFAULT,
  CHARM_LINE_WIDTH_DEFAULT,
  CHARM_POINTER_OFFSET_X_DEFAULT,
  CHARM_POINTER_OFFSET_Y_DEFAULT,
  CHARM_ROPE_LENGTH_DEFAULT,
  CHARM_SEGMENTS_DEFAULT,
  CHARM_STIFFNESS_DEFAULT,
  normalizeCharmRopeConfig,
  type CharmRopeConfig,
} from "~/utils/decoCharm";

/**
 * 挂件组件（`CursorCharm.vue`）的**接线**测试 —— 票 #73。
 *
 * 为什么值得测：`deco.charm` 到组合式参数是逐字段手工赋值的
 * （`ropeLengthPx: Number(charm.rope_length)` 这样十连发）。复制粘贴接错
 * ——比如把静止摆动频率接进摆动幅度——今天没有任何测试会变红，页面上只表现
 * 为"后台调了参数没反应"。组件层此前零覆盖，而 #74 / #75 还要往这条线上
 * 再加三个字段，先把地基按"名字对应"锁住。
 *
 * 桩法（后续票直接复用）：
 *   1. `vi.mock("~/composables/useCursorCharm")` 在这里是**记录用的边界**，不是
 *      把协作方换成空壳：组件真正要交付的东西就是交给它的那份 options，所以
 *      原样录下来断言（与 `JellyLauncher.test.ts` 的既有手法一致）。物理结果
 *      由 `useCursorCharm.test.ts` / `decoCharm.test.ts` 负责。
 *   2. `useSiteStore` 在 `.vue` 里是 Nuxt 自动导入（没有 import 语句），因此用
 *      `vi.stubGlobal("useSiteStore", () => store)` 提供；store 用 `reactive`
 *      包一层，改一个字段后组件内部的 computed 会失效。
 *   3. `useRuntimeConfig` 走 `#imports`（被别名为 `test/mocks/nuxt-imports.ts`），
 *      用 `vi.stubGlobal("__useRuntimeConfigMock", ...)` 给它 `app.baseURL`。
 * 全程不碰真浏览器、不发真实请求。
 *
 * 扩展方式（#74 的 `fps_cap`、#75 的 `pointer_offset_x/y`）：在 `FIELD_WIRING`
 * 里加**一行** `{ field, param, sample, fallback }`，下面逐字段的几条用例都从这张
 * 表生成。`sample` 可以是负数（两个偏移量就是有符号的）。
 *
 * 票 #77 起，"`null` 与 `undefined` 都表示这个键没有值"这件事**只在归一化层**
 * 处理：组件把运行时配置原样（可能是 null）交给 `normalizeCharmRopeConfig`，
 * 由它折成 NaN 再按各自的钳位规则回落默认值。组件层不再自己做 `charmNumber`
 * 之类的兜底 —— 两处兜底一旦分叉，就会重新退回 `Number(null) === 0`（轻晃被
 * 读成"关掉"、指针端被读成"压在热点上"）。对应断言见下面「null 不等于显式 0」
 * 那组用例。 */

interface CapturedCharmOptions {
  configEnabled: () => boolean;
  config: () => Partial<CharmRopeConfig> | null | undefined;
}

// vi.mock 的工厂会被提升到 import 之前，可共享的可变状态必须放 vi.hoisted。
const harness = vi.hoisted(() => ({
  /** 组件交给组合式的那份 options（每次挂载都会被重新记录）。 */
  options: null as CapturedCharmOptions | null,
  /** 组合式交出的门控 ref（模板据此 v-if）；由用例直接翻。 */
  enabled: null as Ref<boolean> | null,
}));

vi.mock("~/composables/useCursorCharm", async () => {
  const { ref } = await import("vue");
  harness.enabled = ref(false);
  return {
    // 只记录 + 返回惰性 ref：本票断言的是"交给组合式的参数"，不是物理结果。
    useCursorCharm: (options: CapturedCharmOptions) => {
      harness.options = options;
      return {
        enabled: harness.enabled,
        visible: ref(false),
        pointer: ref({ x: 0, y: 0 }),
        tetherPath: ref(""),
        logoCenter: ref({ x: 0, y: 0 }),
        tetherGeometry: null,
        physicsFrames: 0,
        dispose: () => {},
      };
    },
  };
});

type CharmBag = Record<string, unknown>;

interface FakeSiteStore {
  deco: { charm?: CharmBag } | null;
  ensurePublicBootstrap: () => Promise<void>;
}

/** 假站点 store：形状与真实 Pinia store 一致（`deco` 是解包后的值，可能是 null）。 */
function makeSiteStore(deco: { charm?: CharmBag } | null): FakeSiteStore {
  return reactive({
    deco,
    ensurePublicBootstrap: () => Promise.resolve(),
  }) as FakeSiteStore;
}

let siteStore: FakeSiteStore = makeSiteStore(null);

function mountCharm(deco: { charm?: CharmBag } | null) {
  siteStore = makeSiteStore(deco);
  const wrapper = mount(CursorCharm);
  const options = harness.options;
  if (!options) throw new Error("挂件没有调用 useCursorCharm：接线整段缺失");
  return { wrapper, options };
}

/** 当前假 store 里的 `deco.charm`（reactive 代理，改了就影响组件）。 */
function charmBag(): CharmBag {
  const charm = siteStore.deco?.charm;
  if (!charm) throw new Error("用例前置：假 store 里没有 deco.charm");
  return charm;
}

/** 读一次组件交出的运行参数；读取本身不许抛错（配置再烂也不该炸页面）。 */
function readConfig(options: CapturedCharmOptions, what: string): Partial<CharmRopeConfig> {
  let params: Partial<CharmRopeConfig> | null | undefined;
  expect(() => {
    params = options.config();
  }, `${what}：读运行参数时抛错了`).not.toThrow();
  return params ?? {};
}

/** 一份"后台把每个参数都填成默认值"的配置；用例只在它上面改一个字段。 */
const DEFAULT_CHARM: CharmBag = {
  enable: true,
  // logo_size 不进组合式参数（它走 clampCharmLogoSize），列在这里只为让夹具像真的。
  logo_size: 48,
  rope_length: CHARM_ROPE_LENGTH_DEFAULT,
  segments: CHARM_SEGMENTS_DEFAULT,
  gravity: CHARM_GRAVITY_DEFAULT,
  damping: CHARM_DAMPING_DEFAULT,
  line_width: CHARM_LINE_WIDTH_DEFAULT,
  idle_sway_hz: CHARM_IDLE_SWAY_HZ_DEFAULT,
  idle_sway_amp: CHARM_IDLE_SWAY_AMP_DEFAULT,
  fps_cap: CHARM_FPS_CAP_DEFAULT,
  pointer_offset_x: CHARM_POINTER_OFFSET_X_DEFAULT,
  pointer_offset_y: CHARM_POINTER_OFFSET_Y_DEFAULT,
};

/** 一份"没有这个键"的 `deco.charm`：模拟老库 / 旧后台根本没发这个字段。 */
function charmWithout(field: string, source: CharmBag = DEFAULT_CHARM): CharmBag {
  return Object.fromEntries(Object.entries(source).filter(([key]) => key !== field));
}

interface FieldWiring {
  /** 后台 `deco.charm` 里的键名。 */
  field: string;
  /** 组件喂给 `useCursorCharm` 的参数名（`CharmRopeConfig` 的键）。 */
  param: keyof CharmRopeConfig;
  /** 取样值：与默认值不同，且每个字段彼此不同 —— 接串了必然有一处对不上。 */
  sample: number;
  /** 归一化后的默认值（独立来源：`decoCharm.ts` 的 `CHARM_*_DEFAULT` 常量表）。 */
  fallback: number;
}

/**
 * 一张表就是全部接线。#74 / #75 加字段时只在这里加一行。
 *
 * `param` 用 `keyof CharmRopeConfig` 而不是字符串：参数名打错、或还没进运行配置
 * 就写进来，类型检查会先红（比运行时断言更早）。
 */
const FIELD_WIRING: FieldWiring[] = [
  { field: "rope_length", param: "ropeLengthPx", sample: 200, fallback: CHARM_ROPE_LENGTH_DEFAULT },
  { field: "segments", param: "segments", sample: 18, fallback: CHARM_SEGMENTS_DEFAULT },
  { field: "gravity", param: "gravity", sample: 3200, fallback: CHARM_GRAVITY_DEFAULT },
  { field: "damping", param: "damping", sample: 0.96, fallback: CHARM_DAMPING_DEFAULT },
  { field: "line_width", param: "lineWidthPx", sample: 5.5, fallback: CHARM_LINE_WIDTH_DEFAULT },
  { field: "idle_sway_hz", param: "idleSwayHz", sample: 1.2, fallback: CHARM_IDLE_SWAY_HZ_DEFAULT },
  { field: "idle_sway_amp", param: "idleSwayAmp", sample: 32, fallback: CHARM_IDLE_SWAY_AMP_DEFAULT },
  // #74：帧率上限。120 与其余六个取样值都不同，接串了会立刻红。
  { field: "fps_cap", param: "fpsCap", sample: 120, fallback: CHARM_FPS_CAP_DEFAULT },
  // #75：指针端偏移。`sample` 故意用负数 —— 这两个字段是有符号的，
  // 一旦接线时被当成"非负参数"处理（负值回落默认）就会立刻对不上。
  // 两轴取样值彼此不同，接串（X 接成 Y）也跑不掉。
  {
    field: "pointer_offset_x",
    param: "pointerOffsetX",
    sample: -6,
    fallback: CHARM_POINTER_OFFSET_X_DEFAULT,
  },
  {
    field: "pointer_offset_y",
    param: "pointerOffsetY",
    sample: -18,
    fallback: CHARM_POINTER_OFFSET_Y_DEFAULT,
  },
  {
    field: "stiffness",
    param: "stiffness",
    sample: 0.7,
    fallback: CHARM_STIFFNESS_DEFAULT,
  },
];

beforeEach(() => {
  harness.options = null;
  if (harness.enabled) harness.enabled.value = false;
  siteStore = makeSiteStore(null);
  vi.stubGlobal("useSiteStore", () => siteStore);
  vi.stubGlobal("__useRuntimeConfigMock", () => ({ app: { baseURL: "/" }, public: {} }));
});

describe("挂件接线：配置字段 → 运行参数（按名字对应）", () => {
  it.each(FIELD_WIRING)("$field → $param：只改这一个字段，只动这一个参数", ({ field, param, sample }) => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
    const before = readConfig(options, `deco.charm.${field} 改值前`);

    charmBag()[field] = sample;
    const after = readConfig(options, `deco.charm.${field} 改值后`);

    expect(
      after[param],
      `字段 deco.charm.${field} 没有接到参数 ${param}（期望 ${sample}，实际 ${String(after[param])}）`,
    ).toBe(sample);

    const drifted = (Object.keys(after) as (keyof CharmRopeConfig)[]).filter(
      (key) => key !== param && after[key] !== before[key],
    );
    expect(
      drifted,
      `只改了 deco.charm.${field}，却连带改了参数 ${drifted.join(" / ")} —— 接线串了`,
    ).toEqual([]);

    wrapper.unmount();
  });

  it("每个字段同时给出不同值时，每个参数各就各位", () => {
    const charm: CharmBag = { ...DEFAULT_CHARM };
    for (const { field, sample } of FIELD_WIRING) charm[field] = sample;
    const { wrapper, options } = mountCharm({ charm });

    const params = readConfig(options, "每个字段同时改值");

    const wrong = FIELD_WIRING.filter(({ param, sample }) => params[param] !== sample).map(
      ({ field, param, sample }) => `${field}→${param} 期望 ${sample} 实际 ${String(params[param])}`,
    );
    expect(wrong, `以下字段的接线不对：\n${wrong.join("\n")}`).toEqual([]);

    wrapper.unmount();
  });

  // 反向守卫：让"接线表"和"组件真正交付的参数"互为镜像。以后有人往组件里加一行
  // 赋值却忘了在 FIELD_WIRING 里加对应断言（#74 / #75 就会这么做），这里直接红。
  it("组件交付的参数集合与接线表一一对应", () => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });

    const delivered = Object.keys(readConfig(options, "参数集合")).sort();
    const expected = FIELD_WIRING.map(({ param }) => param).sort();

    const extra = delivered.filter((key) => !expected.includes(key as keyof CharmRopeConfig));
    const missing = expected.filter((key) => !delivered.includes(key));
    expect(
      { extra, missing },
      `组件交付的参数与接线表不一致：表里没写的参数 ${extra.join(" / ") || "（无）"}；组件没交付的参数 ${missing.join(" / ") || "（无）"}（加字段时记得在 FIELD_WIRING 里加一行）`,
    ).toEqual({ extra: [], missing: [] });

    wrapper.unmount();
  });
});

describe("挂件接线：总开关", () => {
  it("enable 为 true 时总开关打开", () => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM, enable: true } });
    expect(options.configEnabled(), "deco.charm.enable=true 时总开关必须是开的").toBe(true);
    wrapper.unmount();
  });

  it("enable 为 false 时总开关关闭", () => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM, enable: false } });
    expect(options.configEnabled(), "deco.charm.enable=false 时总开关必须是关的").toBe(false);
    wrapper.unmount();
  });

  it("enable 缺失时按关闭处理（缺省不渲染，但不抛错）", () => {
    const { wrapper, options } = mountCharm({ charm: charmWithout("enable") });
    let switchedOn: boolean | undefined;
    expect(() => {
      switchedOn = options.configEnabled();
    }, "deco.charm.enable 缺失时读总开关抛错了").not.toThrow();
    expect(switchedOn, "deco.charm.enable 缺失时总开关必须是关的").toBe(false);

    wrapper.unmount();
  });

  // 总开关是"读的时候现取"而不是挂载时快照：后台改完开关，组件不重建也要跟上
  // （组合式的 switchOn 是个 computed，靠的就是这条）。
  it("挂载后改 enable 立刻生效，不用重建组件", () => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM, enable: true } });
    expect(options.configEnabled(), "初始 enable=true 时总开关必须是开的").toBe(true);

    charmBag().enable = false;
    expect(options.configEnabled(), "把 enable 改成 false 后总开关必须立刻关上").toBe(false);

    charmBag().enable = true;
    expect(options.configEnabled(), "把 enable 改回 true 后总开关必须立刻打开").toBe(true);

    wrapper.unmount();
  });
});

describe("挂件接线：配置没写对时的回落", () => {
  it.each(FIELD_WIRING)("$field 缺失时 $param 回落到默认值且不抛错", ({ field, param, fallback }) => {
    const { wrapper, options } = mountCharm({ charm: charmWithout(field) });

    const params = normalizeCharmRopeConfig(readConfig(options, `deco.charm.${field} 缺失`));

    expect(
      params[param],
      `deco.charm.${field} 缺失时 ${param} 应回落到默认值 ${fallback}，实际 ${params[param]}`,
    ).toBe(fallback);

    wrapper.unmount();
  });

  it.each(FIELD_WIRING)("$field 是非数字时 $param 回落到默认值且不抛错", ({ field, param, fallback }) => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
    charmBag()[field] = "说不清是多少";

    const params = normalizeCharmRopeConfig(readConfig(options, `deco.charm.${field} 非数字`));

    expect(
      params[param],
      `deco.charm.${field} 是非数字时 ${param} 应回落到默认值 ${fallback}，实际 ${params[param]}`,
    ).toBe(fallback);

    wrapper.unmount();
  });

  it.each(FIELD_WIRING)("$field 为 null 时不抛错、归一化后也不是 NaN", ({ field, param }) => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
    charmBag()[field] = null;

    const raw = readConfig(options, `deco.charm.${field} 为 null`);

    // 接线层把 null 原样交出去（"这个键没有值"），由归一化层回落默认值；
    // 但**组合式每次使用前都会归一化**，所以 NaN 绝不该走到 SVG 里。
    let params: Partial<CharmRopeConfig> = {};
    expect(
      () => {
        params = normalizeCharmRopeConfig(raw);
      },
      `deco.charm.${field} 为 null 时归一化抛错了`,
    ).not.toThrow();
    expect(
      Number.isFinite(params[param]),
      `deco.charm.${field} 为 null 时归一化后的 ${param} 必须是有限数值（NaN 进 SVG 会让整条路径消失），实际 ${String(params[param])}`,
    ).toBe(true);

    wrapper.unmount();
  });

  /**
   * 票 #77：`null` 必须回落默认值，不能等同于显式 0。
   *
   * 挑出来的四个字段都走"0 是合法值"的钳位规则 —— 轻晃的两项（0 = 关掉轻晃）
   * 与两轴偏移（0 = 压在热点上）。链路里任何一层若写成 `Number(charm.xxx)`，
   * `Number(null) === 0` 就把"这个键没值"读成"管理员明确填了 0"，读者站的观感
   * 会悄悄变掉（挂件不再轻晃 / 头节点不再偏 3px）。现在兜底只在归一化层，
   * 这组用例守的是"组件原样交出 null + 归一化层兜住"这条完整链路。
   * 其余六个正值参数由钳位规则顺手兜住了，这里不重复断言。
   */
  const NULL_MUST_FALL_BACK = FIELD_WIRING.filter(({ field }) =>
    ["idle_sway_hz", "idle_sway_amp", "pointer_offset_x", "pointer_offset_y"].includes(field),
  );

  it.each(NULL_MUST_FALL_BACK)(
    "$field 为 null 时 $param 回落到默认值 $fallback（null ≠ 显式 0）",
    ({ field, param, fallback }) => {
      const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
      charmBag()[field] = null;

      const params = normalizeCharmRopeConfig(readConfig(options, `deco.charm.${field} 为 null`));

      expect(
        params[param],
        `deco.charm.${field} 为 null 时 ${param} 应回落到默认值 ${fallback}，而不是被 Number(null) 读成 0（实际 ${params[param]}）`,
      ).toBe(fallback);

      wrapper.unmount();
    },
  );

  it.each(NULL_MUST_FALL_BACK)(
    "$field 显式填 0 时 $param 就是 0（回落规则不吃掉显式 0）",
    ({ field, param }) => {
      const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
      charmBag()[field] = 0;

      const params = normalizeCharmRopeConfig(readConfig(options, `deco.charm.${field} 显式 0`));

      expect(
        params[param],
        `deco.charm.${field} 显式填 0 时 ${param} 必须保留 0（关轻晃 / 压在热点上是合法语义）`,
      ).toBe(0);

      wrapper.unmount();
    },
  );

  it.each([
    { label: "deco 整块为 null", deco: null },
    { label: "deco 里没有 charm", deco: {} },
  ])("$label 时每个参数全回落到默认值、总开关视为关闭", ({ label, deco }) => {
    const { wrapper, options } = mountCharm(deco);

    expect(options.configEnabled(), `${label} 时总开关必须视为关闭`).toBe(false);
    const params = normalizeCharmRopeConfig(readConfig(options, label));
    const wrong = FIELD_WIRING.filter(({ param, fallback }) => params[param] !== fallback).map(
      ({ field, param, fallback }) => `${field}→${param} 期望 ${fallback} 实际 ${params[param]}`,
    );
    expect(wrong, `${label} 时这些参数没有回落到默认值：\n${wrong.join("\n")}`).toEqual([]);

    wrapper.unmount();
  });
});

describe("挂件能挂起来（不依赖真浏览器）", () => {
  it("总开关开着时渲染挂绳与站标，关着时整层消失", async () => {
    const { wrapper } = mountCharm({ charm: { ...DEFAULT_CHARM, enable: true } });

    harness.enabled!.value = true;
    await wrapper.vm.$nextTick();
    const layer = wrapper.find(".charm-layer");
    expect(layer.exists(), "总开关开着时必须渲染挂件层").toBe(true);
    expect(layer.attributes("aria-hidden"), "挂件层必须对读屏隐藏").toBe("true");
    expect(layer.find("path.charm-layer__tether").exists(), "挂件层里必须有挂绳路径").toBe(true);
    expect(layer.find("image.charm-layer__logo").exists(), "挂件层里必须有站标").toBe(true);

    harness.enabled!.value = false;
    await wrapper.vm.$nextTick();
    expect(wrapper.find(".charm-layer").exists(), "总开关关着时不该渲染挂件层").toBe(false);

    wrapper.unmount();
  });
});
