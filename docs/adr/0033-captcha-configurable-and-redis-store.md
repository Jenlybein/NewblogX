# ADR-0033：图形验证码参数后台可配置、按入口闸门、存储换 Redis

- 状态：已接受（2026-11-17 落地，实施记录见文末「实施情况」）
- 日期：2026-11-17
- 来源：[#406](https://github.com/Jenlybein/NewblogX/issues/406)（图形验证码可配置化）· 关联 [ADR-0020](./0020-config-section-registry-and-named-sections.md)（配置段注册表与决策 3 的分工先例）· [ADR-0032](./0032-content-filter.md)（内容过滤段先例：仅后台读写、无公开读）

## 背景与问题

图形验证码原来只有**一个开关**（`site.login.captcha`），开关之外的参数全部写死在代码里：

1. **参数冻结在 `handler.go`**：`Height: 60`、`Width: 200`、`NoiseCount: 3`、`ShowLineOptions: 2 | 4`、`Length: 4`、字符集字面量、`color.RGBA{R:10, G:20, B:50, A:10}`，代码注释把理由写得很直白——「沿用旧参数：改这些会让前端已经调好的 input 尺寸对不上，属于无收益的变动」。当时的判断是：这不过是登录框里一张小图，不变就不会坏，改它还要发版，于是冻住。现在这个判断不成立了：有效期（当时根本没有「有效期」概念）要按站点调；脚本变强了要换驱动；手机端希望用纯数字；表单改版要图片尺寸跟着改。这些都是**安全与可用性参数**，不是「无收益的变动」，每改一项都发版不可接受。
2. **字符集里字母重复**：`123456789ABCDEFGHIJKLHIJKLMNOPQRSTUVWXYZ` 中 `IJKL` 拼了两遍。可用字符数被悄悄砍掉一截，等于把穷举难度白降一档；而代码字面量里的重复没人会去数，这个错一直躺在那。
3. **三条路由共用一个全局中间件**：`POST /api/users/login`、`POST /api/users/email/verify`、`POST /api/users/phone/verify` 都挂 `runtimeMw.CaptchaMiddleware`，只有一个总开关。想「只给发码加验证码、登录不加」做不到；想临时拆掉一处防护又只能整体关。而发码被滥用的方式（刷短信，真花钱）与登录被滥用的方式（撞库）本来不同。
4. **存储是进程内内存**：`base64Captcha.DefaultMemStore` 是 `sync.Map`，有两个后果：(a) **重启即失效**——正在登录的人手里那个 `captcha_id` 查不到；(b) **多副本必坏**——A 实例生成、B 实例校验永远失败，而接口层面完全看不出（都返 200，只是永远「验证码错误」）。单实例时两者都不显形，所以它们专挑扩容 / 重启那天一起炸。
5. **响应不带有效期**：前端拿不到 TTL，只能猜「这张图还能用多久」；运营改完有效期也无从看到生效值。
6. **运营看不到「哪些入口正在校验」**：开关是唯一信号，而读者站又要靠它决定每个表单画不画验证码框，于是「图片长什么样」与「哪条路由要校验」两件事被压在同一处，谁都不敢动。

## 决策

1. **新增第 12 段运行时配置 `captcha`，只放渲染与时效参数**；「哪些入口要验证码」留在 `site.login`，新增 `captcha_login` / `captcha_email_verify` / `captcha_phone_verify` 三个布尔（默认全开）。分工依据沿用 [ADR-0020](./0020-config-section-registry-and-named-sections.md) 决策 3 的先例（`sms` 段只放发送凭证、登录策略留在 `site.login`）：**策略留 `site.login`，渲染参数进专属段**。总开关与三个闸门**必须公开可读**（读者站靠 `GET /api/site/site` 决定每个表单渲染不渲染验证码框），而 `captcha` 段**不给公开读**。
2. **字段集合与「不适用按驱动清零」**：`driver`（`string` 字符型 / `math` 算术型 / `digit` 纯数字）、`length`（字符型 2–8、纯数字 3–8）、`width`（80–400）、`height`（30–120）、`expire_seconds`（60–3600，同时是存储 TTL）、`source`（字符型字符集，最多 128 字符）、`noise_count`（0–10）、`show_hollow_line` / `show_slime_line` / `show_sine_line` 三个布尔、`bg_color`（`#RRGGBB` 或 `#RRGGBBAA`）、`max_skew`（0–1）与 `dot_count`（0–20，仅纯数字）。归一化把**该驱动用不上的字段清零**（算术型清 `length` / `source`；纯数字清背景色、噪点与三个干扰线开关），而不是留着让「保存成功但没效果」，也避免切换驱动时旧字段复活成意外行为。后台表单同步禁用这些输入框。
3. **归一化口径：0 / 空串回落默认值，越界直接报错**（错误码 `site.captcha_invalid`，文案点名哪个字段越什么界），**不静默夹取**。与 `deco.charm` 的「越界钳回边界」刻意不同：charm 是渲染手感参数，坏值只是不好看；验证码参数是安全参数——宽度填 5000 会撑爆登录布局、有效期填成 99999 等于给出爆破窗口、字符集去重后只剩 1 个字符等于没有验证码，静默接受等于**静默削弱防护**，而且运营以为存进去了、排查时没有任何线索。「0 / 空串回落默认」与运行时配置 merge 的语义一致（没填不等于填了非法值）。
4. **三种干扰线用三个布尔，不暴露库的位掩码**：`base64Captcha` 的 `OptionShowHollowLine=2` / `SlimeLine=4` / `SineLine=8` 是库的内部编码。让运营记「填 6 就是空心线 + 泥浆线」没道理，魔法数字还会漏进契约与前端；三个布尔让后台是三个开关、契约是三个字段、翻成位值的唯一一处留在 `driver.go` 里。
5. **字符集去空白、去重复字符，不足 2 个字符报错；默认值换成去掉易混字符的那份**。去重必须放进归一化而不是只改一次默认值：只改默认值能修好当前那份，运营自己粘一份带重复的字符集仍会白降难度。默认字符集改为 `23456789ABCDEFGHJKMNPQRSTUVWXYZ`——去掉 `0/O/1/I/L` 这些易混字符，它们只会制造「我看对了却填错」的投诉。
6. **验证码存储从进程内内存换成 Redis**：`captcha.NewRedisStore` 实现 `base64Captcha.Store`，键前缀 `blogx:captcha:`（与其他 Redis 用途共用库，命名空间不能省）。**TTL 每次生成现读配置**（`TTLFunc` 回调，不在构造时快照），所以改有效期不用重启、只影响新生成的图；**Redis 不可用时退回内存存储并打一条 warn**（本地没起 Redis / 单元测试不因此不可用，但日志里要说明「重启即失效、多副本不通用」）。写失败只记错误、不 panic：验证码是锦上添花的手段，Redis 抖动的正确降级是「这张图校验不过、让用户换一张」，不是把登录接口打成 500。生成侧与校验侧**必须是同一个 store 实例**（`WireHTTP` 只创建一个，同时传给 handler 与 `RuntimeOptions`），否则前端拿到的 id 校验时查不到。校验口径与旧 MemoryStore 保持一致：**忽略大小写**（`strings.EqualFold`）、`clear=true` 时**一次性删除**（`DEL`，防同一张图答对一次后被重放）。
7. **路由改挂「每条入口自己的闸门」而不是一个全局中间件**：`CaptchaGate(scope)` 分别挂在三条入口上（`login` / `email_verify` / `phone_verify`）。是否真的校验 = 总开关 `site.login.captcha` **且**该入口闸门打开——总开关仍是「一关全关」，保留「先整体关、再逐条开」的迁移路径。**未知 scope 不校验**：挂载点写错时宁可少校验一次（登录链路会立刻测出来），也不要把一条没登记的路由默认当成「要验证码」，那会让整条链路静默 400。
8. **`GET /api/imagecaptcha` 响应回显 `expire_seconds`**：前端据此知道「多久没操作就主动换一张」，也让运营改完有效期后能在响应里立刻看到生效值，而不是靠翻文档猜。
9. **第 12 段的公开读边界**：只有 `GET /site/admin/captcha`（管理读）与 `PUT /site/captcha`（管理写）；公开读仍只有 `site` / `deco` 两段。图片尺寸、字符集、有效期这些运营参数读者没有必要知道，塞进公开段只是白白扩大攻击面与解释成本。
10. **契约同批重生成**：新增 2 个 path（`/site/admin/captcha`、`/site/captcha`）、2 个 operation、3 个 schema（`SiteSectionCaptchaRequest` / `SiteSectionCaptchaResponse` / `Envelope-…SiteSectionCaptchaResponse`），`scripts/contract-structures.json` 的 `minimumEntries` 按实测同批抬高，判据文件不改。

## 理由与已否决方案

- **否决「把参数塞进 `site` 段」**：`site` 段有匿名可读的 `GET /api/site/site`，塞进去等于把图片尺寸、字符集、有效期发给所有访客；而且「登录与安全」页面会变成既改策略（哪个入口开）又改渲染（图长什么样）的同一份表单，一次保存互相覆盖的风险随字段增加。单独开一段一次解决，也与 `content_filter` 段的形状一致。
- **否决「暴露 `show_line_options` 位掩码」**：位值会漏进契约与后台文案，运营要记二进制组合，改库版本时数字还可能变。三个布尔贵在三行字段，便宜的是一整类解释与文档。
- **否决「静默夹取越界值」**：后台显示「保存成功」，实际存下的不是运营填的值；安全参数被悄悄削弱，而排查时没有任何线索。`deco.charm` 可以夹取，是因为它的失效模式只是难看；验证码的失效模式是「防护变弱且没人知道」——同一个问题在两类参数上答案不同，分界线就是失效模式。
- **否决「继续用 MemoryStore，等出问题再说」**：这两个问题在单实例下永远不显形，一旦显形就是重启或扩容当天；而修复成本只是「一个 store 实现 + 装配处一行判断」，现在改远比故障当天改便宜。多副本时它表现为「永远验证码错误」而不是报错，排查成本远高于实现成本。
- **否决「把三个闸门也放进 `captcha` 段」**：那三个布尔必须公开可读。放进后台专属段后，读者站拿不到它们，只能两个错误二选一——所有表单都画框（用户白填，还以为自己填错了），或都不画（后端却在校验，表现为永远「验证码错误」）。
- **否决「每个入口一套独立图片参数」**：三套参数意味着三份契约形状、三份后台表单与三倍的解释成本；而「图长什么样」与「哪条入口要校验」是两个维度，参数统一 + 闸门分开已经覆盖真实需求。
- **否决「只改默认字符集、不做归一化去重」**：修得了一次默认值，修不了运营下一次粘贴进来的重复字符。
- **否决「TTL 在构造 store 时读一次」**：改有效期就要重启，与仓库既有的 Provider 闭包口径（消费点持有取值函数、不持有值快照）正好相反。

## 代价 / 不做什么

- **改了有效期只影响新生成的图**：手里那张仍按它生成时的 TTL 过期，不会把人当场作废。这是刻意的（正在登录的人不该被打断）。
- **算术型的 `length` / `source` 不适用**：题目固定是两位数运算，归一化把这两项清零；后台禁用对应输入框。
- **纯数字型不支持背景色与干扰线**：库的 `DriverDigit` 只有尺寸、位数、倾斜、背景点五个参数；归一化把这些字段清零，不是「配了但没生效」。
- **切换驱动后已发出的旧图仍按旧驱动校验**：答案以字符串存在 Redis 里，校验只比对答案文本，不重建驱动，所以旧图在 TTL 内照样能校验通过；新图才用新驱动。
- **不含中文点选型**：库没有这个驱动，点选验证码还要图片素材 + 命中坐标 + 另一套校验契约，属于另一件事，本段不做。
- **字符集有上限（128 字符）**：防一次填几万个字符把存储与图片生成拖坏；不是「想填多少填多少」。
- **后端不校验前端禁用状态**：绕过 UI 提交的不适用字段会被清零而不是报错（清零是口径，不是绕过漏洞）。
- **Redis 是新增的存储依赖**：Redis 抖动的那段时间里验证码可能写不进 / 读不到，用户表现为「验证码错误，换一张」；这不是登录阻塞，也不会 500。
- **不做**：验证码主题 / 字体上传、按入口不同的图片参数、验证码通过率统计、音频验证码、图形验证码与内容过滤联动。

## 当前生效约束

1. 运行时配置恰好**十二**段（含 `sms`、`content_filter` 与 `captcha`）：`site / ai / mail / storage / oauth / sms / log / upload / deco / image_gc / content_filter / captcha`；`sectionRegistry` 是唯一事实源，补种、全量解码、缓存应用与写入校验都从它派生。
2. `site.login` 的 `captcha` 总开关与 `captcha_login` / `captcha_email_verify` / `captcha_phone_verify` 三个闸门**必须公开可读**（`GET /api/site/site` 返回）；不得挪进 `captcha` 段或任何后台专属段。
3. `captcha` 段**不得新增公开读**：只有 `GET /site/admin/captcha` 与 `PUT /site/captcha`；公开读面仍只有 `site` / `deco` 两段。
4. 归一化**越界必错**（`site.captcha_invalid`，文案点名字段与区间），不得静默夹取；只有 `0` / 空串回落默认值。
5. 不适用字段**按驱动清零**，后台表单同步禁用；切换驱动不得让旧字段复活成意外行为。
6. Redis 不可用时**必须降级**到进程内存储并打一条 warn；验证码故障不得上升为登录接口 500。
7. **TTL 必须现读运行时配置**，不得在构造 / 启动时缓存；改有效期只影响新生成的图。
8. 校验口径固定：**忽略大小写**、`clear=true` 一次性删除（同一张图不可重放），不得悄悄改成大小写敏感或可多次使用。
9. **不新增第二份验证码存储实现**；生成侧与校验侧必须共用同一个 store 实例。
10. 入口是否校验 = 总开关 `site.login.captcha` **且**该入口闸门；**未知 scope 不校验**；新增需要验证码的入口必须显式挂 `CaptchaGate(scope)` 并补对应闸门字段。
11. 契约改动必须同批重新生成产物，`scripts/contract-structures.json` 的 `minimumEntries` 按实测同批抬高，判据四文件不得为绕开条目变动而改。

## 相关代码与文档

- 配置段：`backend/conf/conf_captcha.go`（`conf.Captcha` 与字段适用表）· `backend/internal/site/app/captcha.go`（`NormalizeCaptcha`、字符集去重、错误码）· `backend/internal/platform/config/registry.go`（`SectionCaptcha`）· `backend/config/site_default_settings.yaml`
- 生成与校验：`backend/internal/platform/captcha/handler.go` · `driver.go`（三驱动与位值翻译）· `color.go`（背景色解析）· `store_redis.go`（`RedisStore` / TTL 现读 / 降级契约）· `backend/internal/platform/http/middleware/captcha_middleware.go`（`CaptchaGate` / `captchaGateEnabled`）· `backend/app/wiring_http.go`（唯一 store 实例与降级 warn）· `backend/internal/platform/http/router/{site_router.go,user_router.go}`
- 契约与后台：`backend/internal/site/adapter/http/{dto_runtime_sections.go,handler_runtime_sections.go}` · `backend/docs/swagger.json` · `scripts/contract-structures.json` · `frontend_admin/app/components/settings/{SettingsCaptcha.vue,SettingsSiteLogin.vue}` · `frontend_admin/app/{pages/site/captcha.vue,services/admin.ts,utils/siteSettings.ts}` · `frontend/app/composables/useCaptchaProof.ts`
- 文档：本 ADR · [ADR-0020](./0020-config-section-registry-and-named-sections.md)（段注册表现共十二段）· [ADR-0032](./0032-content-filter.md)（内容过滤段同形状先例）· [configuration.md](../architecture/configuration.md)（段表、公开读与「越界即拒绝」口径）· [GLOSSARY](../../GLOSSARY.md)（图形验证码）

## 实施情况（2026-11-17）

| 项 | 内容 | 落地形状 / 文件路径 |
| --- | --- | --- |
| 配置段 | 第 12 段 `captcha` 进注册表，默认值补种，具名端点与契约形状 | `backend/conf/conf_captcha.go` · `backend/internal/platform/config/registry.go` · `backend/config/site_default_settings.yaml` · `backend/internal/site/adapter/http/{dto_runtime_sections.go,handler_runtime_sections.go}` · `backend/internal/platform/http/router/site_router.go` |
| 归一化 | 0 / 空串回落默认、越界报 `site.captcha_invalid`、不适用字段按驱动清零、字符集去空白去重（不足 2 报错）、新默认字符集 | `backend/internal/site/app/captcha.go` · `backend/internal/apperr/apperr.go` |
| 三驱动 | 按 `driver` 构建 `DriverString` / `DriverMath` / `DriverDigit`，位掩码翻译只在驱动层 | `backend/internal/platform/captcha/{driver.go,color.go}` |
| 入口闸门 | `site.login` 三个布尔（默认全开）；`CaptchaGate(scope)` 挂三条入口；未知 scope 不校验 | `backend/conf/site/enter.go` · `backend/internal/platform/http/middleware/captcha_middleware.go` · `backend/internal/platform/http/router/user_router.go` · `backend/config/site_default_settings.yaml` |
| Redis 存储 | `RedisStore` 实现 `base64Captcha.Store`，前缀 `blogx:captcha:`、TTL 现读、`clear` 用 `DEL`；Redis 不可用降级内存 + warn；生成/校验共用同一实例 | `backend/internal/platform/captcha/store_redis.go` · `backend/app/wiring_http.go` |
| 响应回显 | `GET /api/imagecaptcha` 新 `expire_seconds`；前端据此主动换图 | `backend/internal/platform/captcha/handler.go` · `frontend/app/composables/useCaptchaProof.ts` |
| 后台页 | 「站点 → 图形验证码」独立编辑页（只管图片与时效；开关与闸门在「登录与安全」页） | `frontend_admin/app/components/settings/SettingsCaptcha.vue` · `frontend_admin/app/pages/site/captcha.vue` · `frontend_admin/app/{services/admin.ts,utils/siteSettings.ts,types/api.ts}` |
| 契约 | 产物实测 166/210/389 → 168/212/392（+2 path / +2 operation / +3 schema，零删除），`minimumEntries` 同批抬高 | `backend/docs/swagger.json` · `scripts/contract-structures.json` |
