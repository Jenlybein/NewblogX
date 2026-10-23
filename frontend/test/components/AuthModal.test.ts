import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const authModalState = vi.hoisted(() => ({
  authModalOpen: true,
  closeAuthModal: vi.fn(),
}));

const formLoadState = vi.hoisted(() => ({
  password: 0,
  emailLogin: 0,
  register: 0,
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

vi.mock("~/components/auth/PasswordLoginForm.vue", () => {
  formLoadState.password += 1;

  return {
    __esModule: true,
    __isTeleport: false,
    default: defineComponent({
      name: "PasswordLoginFormMock",
      emits: ["success"],
      setup(_, { emit }) {
        return () =>
          h(
            "button",
            {
              "data-test": "password-form",
              onClick: () => emit("success"),
            },
            "password",
          );
      },
    }),
  };
});

vi.mock("~/components/auth/EmailLoginForm.vue", () => {
  formLoadState.emailLogin += 1;

  return {
    __esModule: true,
    __isTeleport: false,
    default: defineComponent({
      name: "EmailLoginFormMock",
      setup() {
        return () => h("div", { "data-test": "email-login-form" }, "email-login");
      },
    }),
  };
});

vi.mock("~/components/auth/EmailRegisterForm.vue", () => {
  formLoadState.register += 1;

  return {
    __esModule: true,
    __isTeleport: false,
    default: defineComponent({
      name: "EmailRegisterFormMock",
      setup() {
        return () => h("div", { "data-test": "register-form" }, "register");
      },
    }),
  };
});

async function mountAuthModal() {
  const { default: AuthModal } = await import("~/components/auth/AuthModal.vue");
  return mount(AuthModal);
}

describe("AuthModal", () => {
  beforeEach(() => {
    vi.resetModules();
    authModalState.authModalOpen = true;
    authModalState.closeAuthModal.mockReset();
    formLoadState.password = 0;
    formLoadState.emailLogin = 0;
    formLoadState.register = 0;
  });

  it("loads only the default password form on first open", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    expect(wrapper.get('[data-test="password-form"]').exists()).toBe(true);
    expect(formLoadState.password).toBe(1);
    expect(formLoadState.emailLogin).toBe(0);
    expect(formLoadState.register).toBe(0);
  });

  it("lazy-loads secondary tabs only after the user switches to them", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    const tabs = wrapper.getComponent({ name: "NTabs" });
    tabs.vm.$emit("update:value", "email-login");
    await flushPromises();

    expect(wrapper.get('[data-test="email-login-form"]').exists()).toBe(true);
    expect(formLoadState.emailLogin).toBe(1);
    expect(formLoadState.register).toBe(0);

    tabs.vm.$emit("update:value", "register");
    await flushPromises();
    tabs.vm.$emit("update:value", "email-login");
    await flushPromises();

    expect(wrapper.get('[data-test="register-form"]').exists()).toBe(true);
    expect(formLoadState.emailLogin).toBe(1);
    expect(formLoadState.register).toBe(1);
  });

  it("closes the modal when the active form reports success", async () => {
    const wrapper = await mountAuthModal();
    await flushPromises();

    await wrapper.get('[data-test="password-form"]').trigger("click");

    expect(authModalState.closeAuthModal).toHaveBeenCalledTimes(1);
  });
});
