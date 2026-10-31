# ADR-0014：邮箱验证码"发不出就明说"——放弃静默防枚举

- 状态：已交付
- 日期：2026-10-31
- 来源：线上事故（读者站注册页提示"验证码已发送"，用户始终收不到）

## 背景与问题

2026-10-31，管理员在后台「邮件」Tab 配好腾讯云 SES（密钥、发信地址、模板 ID 全部落库）后，去读者站注册页点「发送验证码」，页面提示"验证码已发送"，邮箱始终没有收到。

排查证据链：

1. 线上日志里那次 `POST /api/users/email/verify` 返回 **200、耗时 8ms**，且请求期间**没有读 `runtime_config_models` 的 mail 段、没有 `邮件发送失败` 错误行**——说明它根本没走到发信那一步（同期真实发信耗时约 260ms）。
2. 同一请求的 gorm trace 是 `SELECT count(*) FROM user_models WHERE email = '...'`；该邮箱在库里不存在。
3. 前端注册表单传的是 **`type: 2`（重置密码）**，正确值是 **1（注册）**；邮箱登录表单传 `type: 1`（应为 4）；绑定邮箱传 `type: 4`（应为 3）。
4. 后端旧 `emailCodeBridge.Send` 的规则是：注册/绑定要求邮箱**未**注册，重置密码/登录要求邮箱**已**注册；不匹配时**不发信，但照样返回成功 + 一个 id**（防枚举）。于是"传 2 + 邮箱不存在"命中"不发"分支 → 静默失效。

也就是说：一次调用方笔误，被后端的静默设计放大成"接口成功、前端提示已发送、零日志、零邮件"。用户拿到的 `id` 在 Redis 里没有对应记录，下一步只会得到"验证码不存在或错误"，彻底卡死。

## 决策

1. **前端只写用途名**：`sendEmailCode({ email, purpose })`，`purpose ∈ register / resetPassword / bindEmail / login`；数字只在 [services/auth.ts](../../frontend/app/services/auth.ts) 一处映射，且取值类型来自契约（swag 从 `binding:"oneof=1 2 3 4"` 直出 `enum`，前端 `Schema<...>["type"]` 派生），不再是手写联合类型。
2. **后端不再"成功但不发"**：邮箱状态与用途不匹配时返回明确错误码——`user.email_registered`（注册但已注册）、`user.email_taken`（绑定但已被占用）、`user.email_unregistered`（登录/重置但未注册）——由前端原样展示。
3. **`type` 加校验**：`binding:"required,oneof=1 2 3 4"`。越界值走参数错误（400），不再一路走到发信层伪装成"邮件发送失败"。
4. **频控挪到状态校验之后**，并给发码单独的错误码 `user.mail_too_frequent`：被拒的请求不再烧掉同一邮箱的发码额度；文案也不再复用"登录失败次数过多"（会把人引向错误的排查方向）。
5. **回归测试**：前端组件测试断言两个表单**真正出网的 body**（注册 1、登录 4）以及映射表四值；后端测试断言四种不匹配都报错且一封信都不发、状态匹配时信确实发出（含模板 action / 主题）。

## 理由与已否决方案

**保留静默的收益被高估了。** 静默防枚举想挡住的是"拿发码接口探测某邮箱是否注册"。但兑换端点（注册/登录/重置/绑定）全在 `EmailVerifyMiddleware` 后面，攻击者没有合法验证码就拿不到"邮箱已被占用"这类文案——也就是说，沉默挡住的信息，正常用户同样拿不到，而正常用户正是被它卡死的那一方。

已否决：

- **只加日志、行为不变**：能排查，但用户仍旧卡在"验证码已发送"上。日志是补充，不是修复。
- **一律真发（不看邮箱状态）**：也不静默，但会给未注册的邮箱发"重置密码/登录"验证码，把错误推迟到下一步才暴露；仍然是"发了信但用不了"。不如当场说明。
- **前端继续传数字、只改对常量**：把"用对常量"交给记忆力，同类笔误还会再来（本次三个调用点全错，不是个别失误）。

## 代价 / 不做什么

- **公开接口现在能被用来判断某个邮箱是否已注册**（枚举面变宽）。个人站点接受这个代价；若要收窄，正解是"状态不符也发一封说明邮件"（需要第二个 SES 模板），而不是退回静默。
- **未做**：把 kind 存进 Redis 验证码记录，并在中间件校验"码的用途与兑换端点一致"。目前同一个码可在四个端点间通用；风险有限（码证明的是邮箱归属，兑换端点用的也是同一个邮箱），但"重置密码的码能在注册页用"仍是一处不一致，留作后续。
- **未做**：保存邮件配置时校验完整性。只提交部分字段的 `PUT /site/mail` 会把 `template_id` / `from_address` 清零却仍返回"配置更新成功"，这是另一类"说成功但没生效"，需要单独一轮处理。

## 当前生效约束

1. 前端禁止在调用点写 `type` 数字，只用 `sendEmailCode({ email, purpose })`。
2. `emailCodeBridge.Send` 不允许存在"返回成功但没有调用 `SendCode`"的路径（`backend/app/user_wiring_test.go` 锁死）。
3. `SendEmailRequest.Type` 的合法值由 `binding:"oneof=1 2 3 4"` 与契约 `enum` 共同保证；改词表必须跑 `make openapi` + 两端 `pnpm openapi:types`（契约漂移门禁会拦）。
4. 发码频控的错误码必须与登录限流分开（`user.mail_too_frequent` vs `auth.too_frequent`）。

## 相关代码与文档

- 代码：[backend/app/user_wiring.go](../../backend/app/user_wiring.go)（`emailCodeBridge.Send`）
- 代码：[backend/internal/auth/adapter/http/dto.go](../../backend/internal/auth/adapter/http/dto.go)（`SendEmailRequest.Type`）
- 代码：[backend/internal/apperr/apperr.go](../../backend/internal/apperr/apperr.go)（三个新错误码）
- 代码：[frontend/app/services/auth.ts](../../frontend/app/services/auth.ts)、[EmailRegisterForm.vue](../../frontend/app/components/auth/EmailRegisterForm.vue)、[EmailLoginForm.vue](../../frontend/app/components/auth/EmailLoginForm.vue)、[studio/settings.vue](../../frontend/app/pages/studio/settings.vue)
- 测试：[frontend/test/components/EmailCodeType.test.ts](../../frontend/test/components/EmailCodeType.test.ts)、[backend/app/user_wiring_test.go](../../backend/app/user_wiring_test.go)
- 文档：[README §2.5 邮件](../../README.md)（配置与验证步骤）
