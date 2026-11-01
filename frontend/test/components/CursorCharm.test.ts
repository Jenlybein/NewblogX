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
  CHARM_ROPE_LENGTH_DEFAULT,
  CHARM_SEGMENTS_DEFAULT,
  normalizeCharmRopeConfig,
  type CharmRopeConfig,
} from "~/utils/decoCharm";

/**
 * 挂件组件（`CursorCharm.vue`）的**接线**测试 —— 票 #73。
 *
 * 为什么值得测：`deco.charm` 到组合式参数是逐字段手工赋值的
 * （`ropeLengthPx: Number(charm.rope_length)` 这样八连发）。复制粘贴接错
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
 * 已知偏差（本票是纯测试票，不改生产代码，见本票报告）：字段为 `null` 时组件
 * 走 `Number(null) === 0`。五个"正值"参数（绳长 / 节点 / 重力 / 阻尼 / 线宽）的
 * 钳位规则把 0 当非法值回落默认；但两个轻晃参数用的是 `clampNonNegativeRange`，
 * 0 是合法值（= 关掉轻晃）——于是 `idle_sway_hz: null` 会被读成"关掉轻晃"，而不是
 * 回落到默认的 0.8Hz。所以 null 那一条只锁"不抛错 + 交出有限数值"，不锁具体取值，
 * 免得把偏差焊死；等哪天组件加了 null 兜底，这条不用改。
 */

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

  it.each(FIELD_WIRING)("$field 为 null 时不抛错、也不交出 NaN", ({ field, param }) => {
    const { wrapper, options } = mountCharm({ charm: { ...DEFAULT_CHARM } });
    charmBag()[field] = null;

    const params = readConfig(options, `deco.charm.${field} 为 null`);

    // 只锁"还能跑"：null 具体落到哪个值见文件头的偏差说明。
    expect(
      Number.isFinite(params[param]),
      `deco.charm.${field} 为 null 时 ${param} 必须是有限数值（NaN 进 SVG 会让整条路径消失），实际 ${String(params[param])}`,
    ).toBe(true);
    expect(
      () => normalizeCharmRopeConfig(params),
      `deco.charm.${field} 为 null 时归一化抛错了`,
    ).not.toThrow();

    wrapper.unmount();
  });

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
