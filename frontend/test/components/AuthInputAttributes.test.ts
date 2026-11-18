import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

/**
 * 认证输入框的移动端属性（#435 / #423 第 26 项）。
 *
 * 只钉对外可见的 input 属性：邮箱唤起邮箱键盘、密码被密码管理器分「当前 / 新建」、
 * 手机号与验证码唤起数字键盘、验证码可自动填充、用户名不被自动大写。
 * 不断言 Naive 内部结构：属性由 NInput 的 `type` 与 `input-props` 传进去，
 * 所以这里把 NInput 替身展开 `inputProps`，验证的就是最终落到 input 上的值。
 */

vi.mock("naive-ui", () => {
  const NForm = defineComponent({
    name: "NForm",
    setup(_, { slots }) {
      return () => h("form", slots.default?.());
    },
  });

  const NFormItem = defineComponent({
    name: "NFormItem",
    props: { label: { type: String, default: "" } },
    setup(props, { slots }) {
      return () => h("div", { "data-label": props.label }, slots.default?.());
    },
  });

  const NInput = defineComponent({
    name: "NInput",
    inheritAttrs: false,
    props: {
      value: { type: String, default: "" },
      type: { type: String, default: "text" },
      placeholder: { type: String, default: "" },
      inputProps: { type: Object, default: () => ({}) },
    },
    emits: ["update:value"],
    setup(props, { attrs, emit }) {
      return () =>
        h("input", {
          ...attrs,
          ...props.inputProps,
          type: props.type,
          value: props.value,
          placeholder: props.placeholder,
          onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
        });
    },
  });

  const NButton = defineComponent({
    name: "NButton",
    props: { disabled: { type: Boolean, default: false } },
    emits: ["click"],
    setup(props, { slots, emit }) {
      return () =>
        h("button", { disabled: props.disabled, onClick: () => emit("click") }, slots.default?.());
    },
  });

  return { NForm, NFormItem, NInput, NButton, useMessage: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }) };
});

vi.mock("~/composables/useCaptchaProof", async () => {
  const { ref } = await import("vue");
  return {
    useCaptchaProof: () => ({
      enabled: ref(false),
      image: ref(""),
      code: ref(""),
      loading: ref(false),
      missing: ref(false),
      refresh: async () => {},
      payload: () => undefined,
    }),
  };
});

beforeEach(() => {
  vi.stubGlobal("useAuthStore", () => ({
    pending: false,
    loginByEmailCode: vi.fn(),
    registerByEmail: vi.fn(),
    loginByPassword: vi.fn(),
    loginByPhone: vi.fn(),
  }));
  vi.stubGlobal("useNuxtApp", () => ({ $api: { request: vi.fn() } }));
});

async function mountForm(name: string) {
  const { default: Form } = await import(`~/components/auth/${name}.vue`);
  return mount(Form);
}

function inputAttributes(wrapper: ReturnType<typeof mount>, index: number) {
  const input = wrapper.findAll("input")[index];
  expect(input, `${name} 的第 ${index} 个输入框不存在`).toBeTruthy();
  return {
    type: input!.attributes("type"),
    autocomplete: input!.attributes("autocomplete"),
    inputmode: input!.attributes("inputmode"),
    autocapitalize: input!.attributes("autocapitalize"),
    spellcheck: input!.attributes("spellcheck"),
  };
}

let name = "";

describe("认证输入属性 · 邮箱", () => {
  it("邮箱登录：邮箱 type=email + autocomplete=email + spellcheck=false，验证码可一次性填充", async () => {
    name = "EmailLoginForm";
    const wrapper = await mountForm(name);

    expect(inputAttributes(wrapper, 0)).toEqual({
      type: "email",
      autocomplete: "email",
      inputmode: undefined,
      autocapitalize: undefined,
      spellcheck: "false",
    });
    expect(inputAttributes(wrapper, 1)).toMatchObject({
      inputmode: "numeric",
      autocomplete: "one-time-code",
    });
  });

  it("邮箱注册：邮箱与验证码同款，密码标记为 new-password", async () => {
    name = "EmailRegisterForm";
    const wrapper = await mountForm(name);

    expect(inputAttributes(wrapper, 0)).toMatchObject({
      type: "email",
      autocomplete: "email",
      spellcheck: "false",
    });
    expect(inputAttributes(wrapper, 1)).toMatchObject({
      inputmode: "numeric",
      autocomplete: "one-time-code",
    });
    expect(inputAttributes(wrapper, 2)).toMatchObject({
      type: "password",
      autocomplete: "new-password",
    });
  });
});

describe("认证输入属性 · 密码与用户名", () => {
  it("密码登录：用户名不自动大写，密码标记为 current-password", async () => {
    name = "PasswordLoginForm";
    const wrapper = await mountForm(name);

    expect(inputAttributes(wrapper, 0)).toMatchObject({
      autocomplete: "username",
      autocapitalize: "none",
    });
    expect(inputAttributes(wrapper, 1)).toMatchObject({
      type: "password",
      autocomplete: "current-password",
    });
  });

  it("手机登录：手机号 type=tel + inputmode=tel + autocomplete=tel，验证码走数字键盘", async () => {
    name = "PhoneAuthForm";
    const wrapper = await mountForm(name);

    expect(inputAttributes(wrapper, 0)).toMatchObject({
      type: "tel",
      inputmode: "tel",
      autocomplete: "tel",
    });
    expect(inputAttributes(wrapper, 1)).toMatchObject({
      inputmode: "numeric",
      autocomplete: "one-time-code",
    });
  });
});
