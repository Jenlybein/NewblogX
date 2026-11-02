import { beforeEach, describe, expect, it, vi } from "vitest";

const siteState = vi.hoisted(() => ({
  runtimeConfig: null as unknown,
  ensurePublicBootstrap: vi.fn(),
}));

const authState = vi.hoisted(() => ({
  currentUser: null as unknown,
}));

vi.mock("~/stores/site", () => ({
  useSiteStore: () => ({
    get runtimeConfig() {
      return siteState.runtimeConfig;
    },
    ensurePublicBootstrap: siteState.ensurePublicBootstrap,
  }),
}));

vi.mock("~/stores/auth", () => ({
  useAuthStore: () => ({
    get currentUser() {
      return authState.currentUser;
    },
  }),
}));

async function createPolicy() {
  vi.resetModules();
  const module = await import("~/composables/useArticleWritePolicy");
  return module.useArticleWritePolicy();
}

function siteConfig(article: Record<string, boolean>) {
  return {
    site_info: { title: "果冻盒" },
    project: { title: "JellyBin" },
    login: {},
    article,
  };
}

/**
 * useArticleWritePolicy 是读者站读站点「发布与展示」开关的唯一入口（#64）。
 *
 * 这里只验证"从站点配置到三个派生值"的接线：配置还没取回时全开、
 * 显式关闭后入口/只读/说明三处同时变化。
 */
describe("useArticleWritePolicy", () => {
  beforeEach(() => {
    siteState.runtimeConfig = null;
    siteState.ensurePublicBootstrap.mockReset();
    authState.currentUser = null;
  });

  it("配置还没取回时按默认值全开，并触发一次公开引导取数", async () => {
    const { policy, entryVisible, readOnly, notices } = await createPolicy();

    expect(policy.value).toEqual({ allowPublish: true, allowDraft: true });
    expect(entryVisible.value).toBe(true);
    expect(readOnly.value).toBe(false);
    expect(notices.value).toEqual([]);
    // 创作页不经过 default/studio 布局，缺了这一步编辑器会永远按"全开"渲染。
    expect(siteState.ensurePublicBootstrap).toHaveBeenCalledTimes(1);
  });

  it("发布开关关闭：入口隐藏，但编辑器仍可写（草稿还开着）", async () => {
    siteState.runtimeConfig = siteConfig({ allow_non_admin_publish: false });
    const { entryVisible, readOnly, notices } = await createPolicy();

    expect(entryVisible.value).toBe(false);
    expect(readOnly.value).toBe(false);
    expect(notices.value).toHaveLength(1);
  });

  it("草稿开关关闭：入口保留、编辑器只读，并说明原因", async () => {
    siteState.runtimeConfig = siteConfig({ allow_non_admin_draft: false });
    const { entryVisible, readOnly, notices } = await createPolicy();

    expect(entryVisible.value).toBe(true);
    expect(readOnly.value).toBe(true);
    expect(notices.value).toHaveLength(1);
    expect(notices.value[0]).toContain("只读");
  });

  it("两个都关：入口隐藏 + 只读，两条说明都在", async () => {
    siteState.runtimeConfig = siteConfig({
      allow_non_admin_publish: false,
      allow_non_admin_draft: false,
    });
    const { entryVisible, readOnly, notices } = await createPolicy();

    expect(entryVisible.value).toBe(false);
    expect(readOnly.value).toBe(true);
    expect(notices.value).toHaveLength(2);
  });

  // 这条钉的是"站点开关只约束非管理员"：后端 `CheckWriteOnCreate/OnUpdate` 对管理员
  // 短路放行，前端如果不知道角色就一律按"被限制"渲染，管理员自己会被锁成只读。
  it("管理员不受两个开关限制：入口保留、编辑器可写、无提示", async () => {
    authState.currentUser = { id: "1", role: 1 };
    siteState.runtimeConfig = siteConfig({
      allow_non_admin_publish: false,
      allow_non_admin_draft: false,
    });
    const { policy, entryVisible, readOnly, notices, isAdmin } = await createPolicy();

    expect(isAdmin.value).toBe(true);
    expect(policy.value).toEqual({ allowPublish: true, allowDraft: true });
    expect(entryVisible.value).toBe(true);
    expect(readOnly.value).toBe(false);
    expect(notices.value).toEqual([]);
  });

  it("普通用户（role=2）仍然被两个开关限制", async () => {
    authState.currentUser = { id: "2", role: 2 };
    siteState.runtimeConfig = siteConfig({
      allow_non_admin_publish: false,
      allow_non_admin_draft: false,
    });
    const { entryVisible, readOnly, isAdmin } = await createPolicy();

    expect(isAdmin.value).toBe(false);
    expect(entryVisible.value).toBe(false);
    expect(readOnly.value).toBe(true);
  });
});
