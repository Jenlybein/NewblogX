import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import AppHeader from "~/components/layout/AppHeader.vue";

/**
 * 页头的组件测试。
 *
 * 为什么值得测：页头是"三档都要能完成四件事"的地方（导航 / 搜索 / 明暗 / 账户），
 * 以前这块只能靠手点。这里用真实组件 + 桩掉 naive-ui / 浏览器外的依赖，
 * 断言结构、无障碍状态与交互接线（*不*断言具体 CSS 类名）。
 */

const FILENAME = "app/components/layout/AppHeader.vue";

// vi.mock 的工厂会被提升到 import 之前，所以可变状态放这里，测试再往里写。
const hub = vi.hoisted(() => {
  const state = {
    themeRef: undefined as { value: "light" | "dark" } | undefined,
    authLoggedIn: undefined as { value: boolean } | undefined,
    push: vi.fn(),
    toggleTheme: vi.fn(),
    openAuthModal: vi.fn(),
    openWriteEntry: vi.fn(),
    // 站点「发布与展示」开关（#64）：默认为空对象 → 读者站按默认值全开。
    siteArticleConfig: {} as Record<string, boolean>,
    authState: {
      initialized: true,
      currentUser: { id: "u1", nickname: "JellyBean" },
      profileId: "u1",
      profileName: "JellyBean",
      profileAvatar: "https://image.example.com/avatar.png",
    },
  };
  return state;
});

vi.mock("~/stores/ui", async () => {
  const { ref } = await import("vue");
  hub.themeRef = ref<"light" | "dark">("light");
  return {
    // 与真实 Pinia store 一致：`useUiStore().theme` 是**解包后的值**（不是 ref），
    // 所以这里用 getter 而不是直接返回 ref。
    useUiStore: () => ({
      get theme() {
        return hub.themeRef!.value;
      },
      toggleTheme: hub.toggleTheme,
      openAuthModal: hub.openAuthModal,
    }),
  };
});

vi.mock("~/stores/auth", async () => {
  const { computed, ref } = await import("vue");
  hub.authLoggedIn = ref(false);
  return {
    useAuthStore: () => ({
      get isLoggedIn() {
        return hub.authLoggedIn!.value;
      },
      initialized: computed(() => true),
      currentUser: computed(() => (hub.authLoggedIn!.value ? hub.authState.currentUser : null)),
      get profileId() {
        return hub.authState.profileId;
      },
      get profileName() {
        return hub.authState.profileName;
      },
      get profileAvatar() {
        return hub.authState.profileAvatar;
      },
      logout: vi.fn(),
    }),
  };
});

vi.mock("~/stores/site", () => ({
  // 与真实 Pinia store 一致：`useSiteStore().runtimeConfig` 是解包后的值。
  useSiteStore: () => ({
    runtimeConfig: {
      project: { abstract: "一点点的个人写作现场" },
      site_info: { subtitle: "" },
      article: hub.siteArticleConfig,
    },
    seo: null,
    aiInfo: null,
    ensurePublicBootstrap: vi.fn(),
  }),
}));

vi.mock("~/composables/useWriteEntry", () => ({
  useWriteEntry: () => ({ openWriteEntry: hub.openWriteEntry }),
}));

const NuxtLinkStub = {
  props: ["to"],
  template: "<a><slot /></a>",
};

const stubs = {
  NuxtLink: NuxtLinkStub,
  ClientOnly: { template: "<div><slot /></div>" },
  NDropdown: { template: "<div><slot /></div>" },
  NSkeleton: { template: '<span data-test="skeleton" />' },
  NAvatar: { template: '<span class="avatar-stub"><slot /></span>' },
};

function mountHeader() {
  return mount(AppHeader, {
    global: { stubs },
  });
}

beforeAll(() => {
  // `#imports` 的 useRoute / useRouter 由 test/mocks/nuxt-imports.ts 提供；
  // 后者通过全局工厂函数注入具体实现（见该文件头部的说明）。
  vi.stubGlobal("__useRouterMock", () => ({ push: hub.push, replace: vi.fn() }));
  vi.stubGlobal("__useRouteMock", () => ({ path: "/", query: {}, fullPath: "/", params: {} }));
});

beforeEach(() => {
  hub.authLoggedIn!.value = false;
  hub.siteArticleConfig = {};
  if (hub.themeRef) hub.themeRef.value = "light";
  vi.stubGlobal("__useRouterMock", () => ({ push: hub.push, replace: vi.fn() }));
  vi.stubGlobal("__useRouteMock", () => ({ path: "/", query: {}, fullPath: "/", params: {} }));
  // Logo 组件要读 app.baseURL 拼品牌资产路径。
  vi.stubGlobal("__useRuntimeConfigMock", () => ({ app: { baseURL: "/" }, public: {} }));
});

describe("页头：结构（三档都成立的必备能力）", () => {
  it("渲染 logo + 字标（旧的 BX 文字方块已退场）", () => {
    const wrapper = mountHeader();

    expect(wrapper.get("header").exists()).toBe(true);
    expect(wrapper.get("img.brand-logo").exists()).toBe(true);
    expect(wrapper.get(".brand-wordmark").text()).toContain("JellyBin");
    expect(wrapper.get(".brand-wordmark").text()).toContain("果冻盒");
    expect(wrapper.text()).not.toMatch(/BX/u);
  });

  it("同时具备导航 / 搜索 / 明暗切换 / 账户四件事", () => {
    const wrapper = mountHeader();
    const nav = wrapper.get('nav[aria-label="主导航"]');

    expect(nav.findAll("a")).toHaveLength(2); // 首页、搜索
    expect(nav.findAll("button")).toHaveLength(1); // 创作
    expect(wrapper.get('form[role="search"]').exists()).toBe(true);
    expect(wrapper.get('input[name="site-search"]').exists()).toBe(true);
    expect(wrapper.get('button[aria-label="切换到暗色主题"]').exists()).toBe(true);
    expect(wrapper.get("[data-chrome=account]").text()).toContain("登录 / 注册");
  });

  it("提供跳到正文的 skip link（键盘用户不必每次 Tab 过整个页头）", () => {
    const wrapper = mountHeader();

    expect(wrapper.get("a[data-chrome=skip-link]").attributes("href")).toBe("#main-content");
  });

  it("页头是吸顶玻璃导航：sticky + 高度压在 56–64px（不再永远遮挡正文）", () => {
    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");

    expect(source).toMatch(/position:\s*sticky/u);
    expect(source).toContain("min-height: var(--site-header-height)");
    // 隐藏/出现只动 transform（不碰会引起重排的属性）。
    expect(source).toMatch(/translateY\(-100%\)/u);
  });

  it("三档响应式：手机折叠 + 平板两行 + 桌面单行（CSS 断点契约）", () => {
    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");

    // 断点分别是 640px（平板起 SKU 两行）与 1024px（桌面单行）。
    expect(source).toMatch(/@media \(min-width: 640px\)/u);
    expect(source).toMatch(/@media \(min-width: 1024px\)/u);
  });
});

describe("页头：手机端折叠菜单（键盘可达）", () => {
  it("默认收起，aria-expanded 为 false", () => {
    const wrapper = mountHeader();

    expect(wrapper.get("button[data-chrome=menu]").attributes("aria-expanded")).toBe("false");
  });

  it("点开后导航与搜索进入可操作状态，再点收起（按钮文案同步）", async () => {
    const wrapper = mountHeader();
    const menuButton = wrapper.get("button[data-chrome=menu]");

    expect(menuButton.attributes("aria-label")).toBe("展开导航菜单");

    await menuButton.trigger("click");
    expect(wrapper.get("button[data-chrome=menu]").attributes("aria-expanded")).toBe("true");
    expect(wrapper.get("button[data-chrome=menu]").attributes("aria-label")).toBe("收起导航菜单");

    await wrapper.get("button[data-chrome=menu]").trigger("click");
    expect(wrapper.get("button[data-chrome=menu]").attributes("aria-expanded")).toBe("false");
  });
});

describe("页头：明暗切换", () => {
  it("点击切换调用站点主题开关（不是组件自己记状态）", async () => {
    const wrapper = mountHeader();

    await wrapper.get("button[aria-label=\"切换到暗色主题\"]").trigger("click");
    expect(hub.toggleTheme).toHaveBeenCalledTimes(1);
  });

  it("暗色时按钮文案变成「切换到浅色主题」（给读屏用户方向感）", () => {
    hub.themeRef!.value = "dark";
    const wrapper = mountHeader();

    expect(wrapper.get('button[aria-label="切换到浅色主题"]').exists()).toBe(true);
  });

  it("亮暗两态使用同一张透底 logo，不随主题切换资产", () => {
    for (const theme of ["light", "dark"] as const) {
      hub.themeRef!.value = theme;
      const wrapper = mountHeader();
      const src = wrapper.get("img.brand-logo").attributes("src");

      expect(src).toContain("logo-256.png");
      expect(src).not.toContain("logo-512-white.png");
    }
  });

  it("源码里没有按主题挑 logo 的逻辑（白瓷砖形态不会回来）", () => {
    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");
    const brandLogoTag = source.match(/<BrandLogo[\s\S]*?\/>/u)?.[0] ?? "";

    expect(brandLogoTag).not.toMatch(/variant/u);
    expect(source).not.toMatch(/logoWhite|logo-512-white/u);
    expect(source).not.toMatch(/theme\s*===\s*["']dark["'][^\n]*["']white["']/u);
  });
});

describe("页头：搜索", () => {
  it("回车提交后跳到既有搜索路由（?key=，路由契约不变）", async () => {
    const wrapper = mountHeader();
    await wrapper.get('input[name="site-search"]').setValue("果冻");
    await wrapper.get('form[role="search"]').trigger("submit");

    expect(hub.push).toHaveBeenCalledWith({ path: "/search", query: { key: "果冻" } });
  });

  it("空关键词只跳搜索页，不拼空参数", async () => {
    const wrapper = mountHeader();
    await wrapper.get('form[role="search"]').trigger("submit");

    expect(hub.push).toHaveBeenCalledWith({ path: "/search", query: {} });
  });

  it("路由 query.key 变化时回填输入框", async () => {
    vi.stubGlobal("__useRouteMock", () => ({ path: "/search", query: { key: "AI" }, fullPath: "/search?key=AI", params: {} }));
    const wrapper = mountHeader();

    expect((wrapper.get('input[name="site-search"]').element as HTMLInputElement).value).toBe("AI");
  });
});

describe("页头：导航与账户", () => {
  it("当前页面在导航里有 aria-current", () => {
    vi.stubGlobal("__useRouteMock", () => ({ path: "/search", query: {}, fullPath: "/search", params: {} }));
    const wrapper = mountHeader();

    expect(wrapper.get('nav[aria-label="主导航"] a[aria-current="page"]').text()).toContain("搜索");
  });

  it("创作入口沿用既有的登录拦截逻辑", async () => {
    const wrapper = mountHeader();

    await wrapper.get('nav[aria-label="主导航"] button').trigger("click");
    expect(hub.openWriteEntry).toHaveBeenCalledTimes(1);
  });

  it("站点关闭「允许非管理员发布文章」后，创作入口不出现（#64）", () => {
    hub.siteArticleConfig = { allow_non_admin_publish: false };
    const wrapper = mountHeader();
    const nav = wrapper.get('nav[aria-label="主导航"]');

    expect(nav.findAll("button")).toHaveLength(0);
    expect(nav.text()).not.toContain("创作");
    // 首页与搜索仍在：开关只影响写作入口，不接管整条导航。
    expect(nav.findAll("a")).toHaveLength(2);
  });

  it("已登录时账户区是菜单按钮，未登录时是登录/注册", async () => {
    hub.authLoggedIn!.value = true;
    const loggedIn = mountHeader();
    expect(loggedIn.get('button[aria-label="打开账户菜单"]').exists()).toBe(true);

    hub.authLoggedIn!.value = false;
    const loggedOut = mountHeader();
    expect(loggedOut.get("[data-chrome=account]").text()).toContain("登录 / 注册");

    await loggedOut.get("[data-chrome=account] button").trigger("click");
    expect(hub.openAuthModal).toHaveBeenCalledTimes(1);
  });
});

/**
 * 滚动方向感知与阈值（组件级）。
 *
 * 判定逻辑在 useScrollChrome（单测覆盖状态机），这里验证页头真的把状态接到
 * class 上：阈值前不动、下滚隐藏、上滚出现、回到顶部退掉玻璃。
 */
describe("页头：滚动方向感知与阈值", () => {
  function setScrollY(value: number) {
    Object.defineProperty(window, "scrollY", { configurable: true, writable: true, value });
  }

  async function scrollTo(value: number) {
    setScrollY(value);
    window.dispatchEvent(new Event("scroll"));
    await nextTick();
  }

  afterEach(() => setScrollY(0));

  it("阈值之前保持静止：不转玻璃、不隐藏", async () => {
    const wrapper = mountHeader();
    await scrollTo(4);

    const header = wrapper.get("header");
    expect(header.classes()).not.toContain("is-scrolled");
    expect(header.classes()).not.toContain("is-hidden");
    expect(header.classes()).not.toContain("glass-thin");
  });

  it("下滚超阈值：转玻璃 + 隐藏；上滚超过迟滞量后重新出现", async () => {
    const wrapper = mountHeader();
    const header = wrapper.get("header");

    await scrollTo(160);
    expect(header.classes()).toContain("is-scrolled");
    expect(header.classes()).toContain("glass-thin");
    expect(header.classes()).toContain("is-hidden");

    await scrollTo(90);
    expect(header.classes()).not.toContain("is-hidden");
    expect(header.classes()).toContain("is-scrolled");
  });

  it("回到顶部：玻璃退掉且页头永远可见", async () => {
    const wrapper = mountHeader();
    const header = wrapper.get("header");

    await scrollTo(160);
    await scrollTo(0);

    expect(header.classes()).not.toContain("is-scrolled");
    expect(header.classes()).not.toContain("is-hidden");
  });
});
