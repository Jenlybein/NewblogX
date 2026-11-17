import { describe, expect, it } from "vitest"

import { contactText, loginPlaceText } from "~/utils/userContact"

// 这一行的取值规则直接决定管理员能不能核对"是不是同一个人"，
// 所以边界都钉住：只有手机号、只有邮箱、两个都有、两个都没有。
describe("contactText", () => {
  it("两个都有时按「手机号 · 邮箱」排列", () => {
    expect(contactText({ phone: "13800001234", email: "a@b.com" })).toBe("13800001234 · a@b.com")
  })

  it("只有手机号时只显示手机号", () => {
    expect(contactText({ phone: "13800001234", email: "" })).toBe("13800001234")
  })

  it("只有邮箱时只显示邮箱", () => {
    expect(contactText({ phone: null, email: "a@b.com" })).toBe("a@b.com")
  })

  it("两个都没有时给明确的「未绑定」，而不是空串", () => {
    // 空串会让"确实没绑"和"渲染坏了"在界面上长得一样 —— 这正是这条断言的理由。
    expect(contactText({})).toBe("未绑定手机号 / 邮箱")
    expect(contactText({ phone: null, email: null })).toBe("未绑定手机号 / 邮箱")
  })
})

describe("loginPlaceText", () => {
  it("IP 与地区都有时用 · 连接", () => {
    expect(loginPlaceText({ ip: "1.2.3.4", addr: "广东省深圳市" })).toBe("1.2.3.4 · 广东省深圳市")
  })

  it("只有 IP 时只显示 IP", () => {
    expect(loginPlaceText({ ip: "1.2.3.4", addr: null })).toBe("1.2.3.4")
  })

  it("都没有时给 —", () => {
    expect(loginPlaceText({})).toBe("—")
  })
})
