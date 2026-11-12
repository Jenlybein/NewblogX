import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const authModalState = vi.hoisted(() => ({
  authModalOpen: true,
  closeAuthModal: vi.fn(),
}));

/** 站点登录开关：默认 tab 由它决定，所以测试要能改。 */
const siteState = vi.hoisted(() => ({
  phoneLogin: true,
}));

vi.mock("naive-ui", () => {
  const NModal = defineComponent({
    name: "NModal",
    props: {
      show: {
        type: Boolean,
        default: false,
      },
    },
    emits: ["update:show"],
    setup(props, { slots }) {
      return () => (props.show ? h("div", { "data-test": "auth-modal" }, slots.default?.()) : null);
    },
  });

  const NCard = defineComponent({
    name: "NCard",
    setup(_, { slots }) {
      return () => h("section", { "data-test": "auth-card" }, slots.default?.());
    },
  });

  const NTabs = defineComponent({
    name: "NTabs",
    emits: ["update:value"],
    setup(_, { slots }) {
      return () => h("div", { "data-test": "auth-tabs" }, slots.default?.());
    },
  });

  const NTabPane = defineComponent({
    name: "NTabPane",
    props: {
      name: {
        type: String,
        required: true,
      },
    },
    setup(props, { slots }) {
      return () => h("div", { "data-tab-name": props.name }, slots.default?.());
    },
  });

  return {
    NModal,
    NCard,
    NTabs,
    NTabPane,
  };
});

vi.mock("~/stores/ui", () => ({
  useUiStore: () => authModalState,
}));

vi.mock("~/stores/site", () => ({
  useSiteStore: () => ({
    runtimeConfig: {
      login: {
        phone_login: siteState.phoneLogin,
        username_pwd_login: true,
        email_login: true,
      },
    },
  }),
}));

vi.mock("~/components/auth/PhoneAuthForm.vue", () => ({
  __esModule: true,
  __isTeleport: false,
  default: defineComponent({
    name: "PhoneAuthFormMock",
    emits: ["success"],
    setup(_, { emit }) {
      return () =>
        h(
          "button",
          {
            "data-test": "phone-form",
            onClick: () => emit("success"),
          },
          "phone",
        );
    },
  }),
}));

vi.mock("~/components/auth/PasswordLoginForm.vue", () => ({
  __esModule: true,
  __isTeleport: false,
  default: defineComponent({
    name: "PasswordLoginFormMock",
    setup() {
      return () => h("div", { "data-test": "password-form" }, "password");
    },
  }),
}));

vi.mock("~/components/auth/EmailLoginForm.vue", () => ({
  __esModule: true,
  __isTeleport: false,
  default: defineComponent({
    name: "EmailLoginFormMock",
    setup() {
      return () => h("div", { "data-test": "email-login-form" }, "email-login");
    },
  }),
}));

async function mountAuthModal() {
  const { default: AuthModal } = await import("~/components/auth/AuthModal.vue");
  return mount(AuthModal);
}

/**
 * 断言口径说明：这里只看 **DOM 里渲染了哪个表单**，不数"模块被 import 了几次"。
 *
 * 数 import 次数要靠 mock 工厂里的计数器，但 vitest 的模块缓存让同一文件里
 * 第二次 import 不再执行工厂——计数器因此会随"这个测试排第几个"而变，
 * 是典型的脆弱断言。渲染结果才是调用方真正能观察到的行为。
 */
describe("AuthModal", () => {
  beforeEach(() => {
    vi.resetModules();
    authModalState.authModalOpen = true;
    authModalState.closeAuthModal.mockReset();
    siteState.phoneLogin = true;
  });

  // 手机号是站点的注册入口，所以站点开着它时默认落在手机号 tab。
  it("默认 tab 是手机号登录", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    expect(wrapper.get('[data-test="phone-form"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="password-form"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="email-login-form"]').exists()).toBe(false);
  });

  // 站点没开「手机号登录」时（默认配置就是关的），默认 tab 必须退回密码登录：
  // 否则访客一开弹窗就吃到「站点未启用手机号登录」，注册与登录入口一起坏掉。
  it("站点关掉手机号登录时，默认 tab 退回密码登录", async () => {
    siteState.phoneLogin = false;
    const wrapper = await mountAuthModal();
    await flushPromises();

    expect(wrapper.get('[data-test="password-form"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="phone-form"]').exists()).toBe(false);
  });

  it("次要 tab 在用户切过去之前不加载", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    expect(wrapper.find('[data-test="email-login-form"]').exists()).toBe(false);
    expect(wrapper.find('[data-test="password-form"]').exists()).toBe(false);

    const tabs = wrapper.getComponent({ name: "NTabs" });
    tabs.vm.$emit("update:value", "email-login");
    await flushPromises();

    expect(wrapper.get('[data-test="email-login-form"]').exists()).toBe(true);

    tabs.vm.$emit("update:value", "password");
    await flushPromises();

    expect(wrapper.get('[data-test="password-form"]').exists()).toBe(true);
  });

  it("当前表单报告成功后关闭弹窗", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    await wrapper.get('[data-test="phone-form"]').trigger("click");

    expect(authModalState.closeAuthModal).toHaveBeenCalledTimes(1);
  });
});
