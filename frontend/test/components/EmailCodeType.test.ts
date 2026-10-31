// 锁死"发送邮箱验证码"实际发出去的 `type`（契约字段，语义见
// backend/internal/auth/adapter/http/dto.go 的 SendEmailRequest）：
//   1 注册、2 重置密码、3 绑定邮箱、4 邮箱登录。
//
// 为什么必须锁死：后端按 type 决定**发不发信**（注册/绑定要求邮箱不存在，
// 登录/重置要求邮箱存在，见 backend/app/user_wiring.go 的 emailCodeBridge.Send）。
// 不匹配时后端照样返回 200 + 一个 id（防枚举），前端于是提示"验证码已发送"，
// 但一封信都不会发出 —— 2026-10-31 线上"注册收不到验证码"正是这么来的
// （注册表单当时传的是 type=2 重置密码）。
//
// 用例断言的是真正出网的请求体，而不是 sendEmailCode 的入参，
// 所以 services/auth.ts 里那张"用途 → type"的映射表也被覆盖到了。
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn(async () => ({ id: "code-1" })));
const messageMock = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}));

// naive-ui 只保留"能挂载、能点按钮、能往输入框里打字"的最小实现。
vi.mock("naive-ui", () => {
  const NForm = defineComponent({
    setup(_, { slots }) {
      return () => h("form", slots.default?.());
    },
  });
  const NFormItem = defineComponent({
    setup(_, { slots }) {
      return () => h("div", slots.default?.());
    },
  });
  const NInput = defineComponent({
    props: { value: { type: String, default: "" } },
    emits: ["update:value"],
    setup(props, { emit }) {
      return () =>
        h("input", {
          value: props.value,
          onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
        });
    },
  });
  const NButton = defineComponent({
    props: { disabled: { type: Boolean, default: false } },
    setup(props, { slots, attrs }) {
      return () => h("button", { ...attrs, disabled: props.disabled }, slots.default?.());
    },
  });
  return { NForm, NFormItem, NInput, NButton, useMessage: () => messageMock };
});

/** 表单里 sendEmailCode 走的是 `useNuxtApp().$api.request`，这里只拦这一层。 */
function stubRequest() {
  vi.stubGlobal("useNuxtApp", () => ({ $api: { request: requestMock } }));
  vi.stubGlobal("useAuthStore", () => ({
    pending: false,
    loginByEmailCode: vi.fn(),
    registerByEmail: vi.fn(),
  }));
}

/** 填邮箱 → 点"发送验证码"，返回这次请求的 [url, options]。 */
async function sendCodeFrom(componentName: "EmailRegisterForm" | "EmailLoginForm") {
  const { default: Form } = await import(`~/components/auth/${componentName}.vue`);
  const wrapper = mount(Form);

  await wrapper.get("input").setValue("someone@example.com");
  await wrapper.get("button").trigger("click");
  await flushPromises();

  expect(requestMock).toHaveBeenCalledTimes(1);
  return requestMock.mock.calls[0] as unknown as [
    string,
    { body: { email: string; type: number } },
  ];
}

describe("发送邮箱验证码的 type", () => {
  beforeEach(() => {
    requestMock.mockClear();
    messageMock.success.mockClear();
    messageMock.error.mockClear();
    stubRequest();
  });

  it("注册表单按「注册」发码（type=1）", async () => {
    const [url, options] = await sendCodeFrom("EmailRegisterForm");
    expect(url).toBe("/api/users/email/verify");
    expect(options.body).toEqual({ email: "someone@example.com", type: 1 });
  });

  it("邮箱登录表单按「邮箱登录」发码（type=4）", async () => {
    const [url, options] = await sendCodeFrom("EmailLoginForm");
    expect(url).toBe("/api/users/email/verify");
    expect(options.body).toEqual({ email: "someone@example.com", type: 4 });
  });

  it("四个用途各自映射到契约里的数字，且没有两个用途重号", async () => {
    const { sendEmailCode } = await import("~/services/auth");
    const purposes = ["register", "resetPassword", "bindEmail", "login"] as const;

    const types: number[] = [];
    for (const purpose of purposes) {
      requestMock.mockClear();
      await sendEmailCode({ email: "someone@example.com", purpose });
      types.push(requestMock.mock.calls[0][1].body.type as number);
    }

    expect(types).toEqual([1, 2, 3, 4]);
    expect(new Set(types).size).toBe(purposes.length);
  });
});
