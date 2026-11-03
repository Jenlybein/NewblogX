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

// 历史入口收进头像菜单的那一档视口（与组件里的查询保持一致）。
const HISTORY_MENU_QUERY = "(min-width: 640px) and (max-width: 1279px)";

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
    // 消息中心未读汇总（#239）：页头入口订阅 useMessageStore。
    messageSummary: {
      comment_msg_count: 3,
      digg_favor_msg_count: 0,
      private_msg_count: 2,
      system_msg_count: 5,
      global_msg_count: 1,
    },
    messageFetched: false,
    refreshSummary: vi.fn(),
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

vi.mock("~/stores/message", () => ({
  useMessageStore: () => ({
    get summary() {
      return hub.messageSummary;
    },
    get totalUnread() {
      const summary = hub.messageSummary;
      return (
        summary.comment_msg_count +
        summary.digg_favor_msg_count +
        summary.private_msg_count +
        summary.system_msg_count +
        summary.global_msg_count
      );
    },
    get fetched() {
      return hub.messageFetched;
    },
    refreshSummary: hub.refreshSummary,
  }),
}));

vi.mock("~/stores/site", () => ({
  // 与真实 Pinia store 一致：`useSiteStore().runtimeConfig` 是解包后的值。
  useSiteStore: () => ({
    runtimeConfig: {
      project: { abstract: "一点点的个人写作现场" },
      site_info: { beian: "" },
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
  template: '<a :href="to"><slot /></a>',
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
  hub.messageFetched = false;
  hub.messageSummary = {
    comment_msg_count: 3,
    digg_favor_msg_count: 0,
    private_msg_count: 2,
    system_msg_count: 5,
    global_msg_count: 1,
  };
  if (hub.themeRef) hub.themeRef.value = "light";
  // restoreMocks 会把 vi.fn 的实现也还原掉：每个用例重新承诺 refreshSummary 返回 Promise。
  hub.refreshSummary.mockResolvedValue({});
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

    expect(nav.findAll("a")).toHaveLength(4); // 首页、搜索、合集、项目
    expect(nav.findAll("button")).toHaveLength(0); // 创作已移到右侧动作区
    expect(wrapper.get('form[role="search"]').exists()).toBe(true);
    expect(wrapper.get('input[name="site-search"]').exists()).toBe(true);
    expect(wrapper.get('button[aria-label="切换到暗色主题"]').exists()).toBe(true);
    expect(wrapper.get("[data-chrome=account]").text()).toContain("登录 / 注册");
  });

  it("导航条是纯链接：首页 / 搜索 / 合集 / 项目，顺序固定", () => {
    const wrapper = mountHeader();
    const labels = wrapper
      .get("nav.site-nav")
      .findAll("a")
      .map((link) => link.text());

    expect(labels).toEqual(["首页", "搜索", "合集", "项目"]);
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

    await wrapper.get('button[data-chrome="write"]').trigger("click");
    expect(hub.openWriteEntry).toHaveBeenCalledTimes(1);
  });

  it("站点关闭「允许非管理员发布文章」后，创作入口不出现（#64）", () => {
    hub.siteArticleConfig = { allow_non_admin_publish: false };
    const wrapper = mountHeader();
    const nav = wrapper.get('nav[aria-label="主导航"]');

    expect(nav.findAll("button")).toHaveLength(0);
    expect(nav.text()).not.toContain("创作");
    // 导航其余四项仍在：开关只影响写作入口，不接管整条导航。
    expect(nav.findAll("a")).toHaveLength(4);
    // 创作已不在导航条里，门控直接让动作区那一项消失。
    expect(wrapper.find('button[data-chrome="write"]').exists()).toBe(false);
    expect(wrapper.get(".site-header__actions").text()).not.toContain("创作");
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
describe("页头：右侧动作区（#198 创作移位）", () => {
  it("顺序固定：亮暗切换 → 创作 → 个人信息 → 手机菜单（按 DOM 顺序断言）", () => {
    const wrapper = mountHeader();
    const order = wrapper
      .findAll(".site-header__actions > *")
      .map((node) => node.attributes("data-chrome") ?? "?unknown");

    expect(order).toEqual(["theme", "write", "account", "menu"]);
  });

  it("窄屏降级为纯图标：640px 起显示「创作」文字，更窄只留图标并带无障碍文案", () => {
    const wrapper = mountHeader();
    const write = wrapper.get('button[data-chrome="write"]');

    // 无障碍文案与悬浮提示不随断点消失。
    expect(write.attributes("aria-label")).toBe("创作");
    expect(write.attributes("title")).toBe("创作");
    expect(write.find(".site-header__write-label").text()).toBe("创作");

    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");
    // 默认（窄屏）隐藏文字；640px 断点起显示（与页头其余断点一致）。
    expect(source).toMatch(/\.site-header__write-label\s*\{\s*display:\s*none;/u);
    const tabletMedia = source.slice(
      source.indexOf("@media (min-width: 640px)"),
      source.indexOf("@media (min-width: 1024px)"),
    );
    expect(tabletMedia).toMatch(/\.site-header__write-label\s*\{\s*display:\s*inline;/u);
  });
});

/**
 * 消息中心入口（#239）。
 *
 * 结构验收：
 *   - 登录后动作区出现入口；未登录不出现（/messages 是 auth 页面）；
 *   - hover / 键盘聚焦展开五个子项，子项右侧显示未读数，0 不显示；
 *   - <640px 用折叠面板里的同一份子项（DOM 常驻、CSS 断点切换可见性）；
 *   - <1024px 仅图标 + 红点（文字由断点隐藏，红点在 totalUnread > 0 时渲染）。
 * 无未读时入口文案保持「消息中心」，不显示 0。
 */
describe("页头：消息中心入口（#239）", () => {
  function login() {
    hub.authLoggedIn!.value = true;
    return mountHeader();
  }

  it("未登录不渲染消息中心入口（消息中心是登录后才会去的页面）", () => {
    const wrapper = mountHeader();

    expect(wrapper.find('[data-chrome="messages"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("消息中心");
  });

  it("登录后动作区出现「消息中心」，带总未读数文案与红点", () => {
    const wrapper = login();
    const entry = wrapper.get('[data-chrome="messages"] .site-header__messages');

    expect(entry.text()).toContain("消息中心");
    expect(entry.attributes("aria-label")).toBe("消息中心（11 条未读）");
    expect(entry.attributes("aria-haspopup")).toBe("menu");
    expect(entry.find(".site-header__dot").exists()).toBe(true);
  });

  it("没有未读时不显示红点，也不显示数字 0", () => {
    hub.messageSummary = {
      comment_msg_count: 0,
      digg_favor_msg_count: 0,
      private_msg_count: 0,
      system_msg_count: 0,
      global_msg_count: 0,
    };
    const wrapper = login();

    expect(wrapper.find(".site-header__dot").exists()).toBe(false);
    expect(wrapper.get(".site-header__messages").attributes("aria-label")).toBe("消息中心");
    expect(wrapper.get(".site-header__messages").text()).toBe("消息中心");
  });

  it("hover 展开五子项面板：顺序固定、子项带自己的未读数、0 不显示", async () => {
    const wrapper = login();

    await wrapper.get('[data-chrome="messages"]').trigger("pointerenter");

    const panel = wrapper.get(".site-messages__panel");
    expect(panel.attributes("role")).toBe("menu");
    const items = panel.findAll(".site-messages__item");
    expect(items).toHaveLength(5);
    expect(items.map((item) => item.text().replace(/\d+/gu, "").trim())).toEqual([
      "评论与回复",
      "点赞与收藏",
      "系统通知",
      "全局通知",
      "私信",
    ]);
    expect(items.map((item) => item.find(".site-messages__count").exists())).toEqual([
      true,
      false,
      true,
      true,
      true,
    ]);
    expect(items[0]!.attributes("href")).toBe("/messages/comments");
    expect(items[4]!.attributes("href")).toBe("/messages/chat");

    await wrapper.get('[data-chrome="messages"]').trigger("pointerleave");
    expect(wrapper.find(".site-messages__panel").exists()).toBe(false);
  });

  it("键盘聚焦也展开面板（hover 之外的可达路径）", async () => {
    const wrapper = login();

    await wrapper.get('[data-chrome="messages"]').trigger("focusin");
    expect(wrapper.find(".site-messages__panel").exists()).toBe(true);
  });

  it("<640px 折叠面板里有同一份五子项入口（DOM 常驻，靠断点切换显示）", () => {
    const wrapper = login();
    const mobile = wrapper.get(".site-messages-mobile");
    const links = mobile.findAll("a");

    expect(links).toHaveLength(5);
    expect(links.map((link) => link.attributes("href")).slice(0, 2)).toEqual([
      "/messages/comments",
      "/messages/digg",
    ]);
    expect(mobile.find(".site-messages__count").exists()).toBe(true);
  });

  it("登录后动作区顺序：亮暗 → 创作 → 消息中心 → 历史 → 个人信息 → 手机菜单", () => {
    const wrapper = login();
    const order = wrapper
      .findAll(".site-header__actions > *")
      .map((node) => node.attributes("data-chrome") ?? "?unknown");

    expect(order).toEqual(["theme", "write", "messages", "history", "account", "menu"]);
  });

  it("登录后若还没拉过未读汇总，页头挂载时主动刷新一次", () => {
    login();

    expect(hub.refreshSummary).toHaveBeenCalledTimes(1);
  });

  it("已经拉过一次就不再重复请求", () => {
    hub.messageFetched = true;
    login();

    expect(hub.refreshSummary).not.toHaveBeenCalled();
  });
});

describe("页头：历史入口（第 241 票）", () => {
  function login() {
    hub.authLoggedIn!.value = true;
    return mountHeader();
  }

  it("未登录不渲染历史入口（/history 是 auth 页面）", () => {
    const wrapper = mountHeader();

    expect(wrapper.find('[data-chrome="history"]').exists()).toBe(false);
    expect(wrapper.find(".site-history-mobile").exists()).toBe(false);
  });

  it("登录后动作区是普通链接：指向 /history，没有悬停面板、没有 aria-haspopup", () => {
    const wrapper = login();
    const entry = wrapper.get('[data-chrome="history"]');

    expect(entry.attributes("href")).toBe("/history");
    expect(entry.attributes("aria-label")).toBe("浏览历史");
    expect(entry.attributes("aria-haspopup")).toBeUndefined();
    expect(entry.text()).toContain("历史");
    // 普通链接：指针进入不会展开任何面板（与消息中心不同）。
    expect(wrapper.find(".site-messages__panel").exists()).toBe(false);
  });

  it("<640px 折叠面板里有同一个入口（DOM 常驻，靠断点切换显示）", () => {
    const wrapper = login();
    const mobile = wrapper.get(".site-history-mobile");

    expect(mobile.find("a").attributes("href")).toBe("/history");
    expect(mobile.text()).toContain("浏览历史");

    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");
    const narrowMedia = source.slice(source.indexOf("@media (max-width: 639px)"));
    expect(narrowMedia).toMatch(/\.site-history-mobile\s*\{\s*display:\s*flex;/u);
  });

  it("640–1279px 历史收进头像菜单：条件项 + 选择后跳 /history（源码门禁）", () => {
    // 菜单选项是 NDropdown 的数据、渲染在浮层里，组件测试观测不到；
    // 与"窄屏降级为纯图标"一样改用源码门禁，锁住三件事：
    // 查询区间、条件项、以及选中后的去向。
    const source = readFileSync(join(process.cwd(), FILENAME), "utf8");

    expect(source).toContain(HISTORY_MENU_QUERY);
    expect(source).toMatch(/if \(historyMenuVisible\.value\) \{/u);
    expect(source).toContain('label: "浏览历史"');
    expect(source).toContain('key: "history"');
    expect(source).toMatch(/key === "history"[\s\S]{0,80}navigateTo\("\/history"\)/u);

    // 动作区链接默认隐藏、1280 断点起才显示：窄屏只留菜单里的那一份。
    expect(source).toMatch(/\.site-header__history\s*\{\s*display:\s*none;/u);
    const wideMedia = source.slice(source.indexOf("@media (min-width: 1280px)"));
    expect(wideMedia).toMatch(/\.site-header__history\s*\{\s*display:\s*inline-flex;/u);
  });
});

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

