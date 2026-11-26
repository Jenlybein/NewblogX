# ADR-0034：图片展示样式与审核闸门

- 状态：已接受
- 日期：2026-11-27
- 来源：图片样式派发与内容审核闭环（票 409–417）· 关联 [ADR-0013](./0013-banner-framing-as-parameters.md)（上传前定形，本 ADR 沿用其"只上传定形后的新文件"）· 违规计分账本在 `moderation` 模块，本 ADR 只决定"什么时候记"
- 术语：[GLOSSARY · 图片与审核](../../GLOSSARY.md)

## 背景与问题

七牛对象存储上的图片，原来只有"传上去就能用"这一条路。四个问题叠在一起：

1. **没有任何审核结论**：违规图（广告、色情、暴恐、敏感人物）传上去就能当头像、封面、正文插图、私信图展示，运营只能事后看到。
2. **七牛的内容审核不可依赖**：七牛的机审结论走两条通道（我们自己调的「主动审核」与七牛回调），回调可能永远不来（域名被封期、控制台漏配回调地址），而"没结论"在当时等价于"放行"。
3. **展示地址不受控**：图片行上存的是 CDN 直链，只要行存在，任何人都能拿到它。于是"先展示、等审核结论再回撤"在技术上不成立——CDN 已经缓存过那张图了。
4. **上传入口分散在两端**：读者站有头像、封面、正文插图、私信图，后台有轮播封面、合集封面。每个入口各写一套"传七牛 + 建记录"，任何一条改漏都会绕开审核。

另外两件相关的事：图片展示需要按尺寸派发样式（列表小图、详情大图、头像方图），而**探测图片真实格式与尺寸**这件事本身也有坑——见决策 9。

## 决策

1. **服务端只有一个入库点**。六个上传入口全部只调 `POST /api/images/upload-tasks`，七牛直传（浏览器 → 七牛，不经服务器），回调到达后进 `media/app/persist()`；`persist()` 是全仓唯一写 `image_models` 的地方。新入口只要走这套 API 就自动被审核，不存在"某入口漏审"的代码路径。

2. **图片状态是一个七态机，短名进契约**：`unknown(0) / pass(1) / deleted(2) / orphaned(3) / review(4) / block(5) / pending(6)`。`pending` 与 `review` 都不可展示，区别在来源：`pending` 是"还没有任何结论"（超时扫描只盯它），`review` 是"机审疑似、等人工"。`block` 是终局：任何来源都不能把它改回 `pass`。

3. **未过审的图不给地址**。`uploadVisibleURL(status, url)` 在状态不是 `pass` 时返回空串，前端据此画"审核中"占位。这是决策 3 与决策 4 的前提：不先把地址发出去，就不存在"回撤已经缓存过的图"。

4. **审核结论有三条通道，取最严**：
   - ① 入库后**同步**调七牛机审（`CensorNow`，`POST ai.qiniuapi.com/v3/image/censor`）。同步是有意的：私信发图、设置封面需要"立刻知道能不能用"。
   - ② 七牛回调 `POST /api/images/qiniu/audit/callback`（异步，结论可能与 ① 不同）。
   - ③ 超时扫描：每 1 分钟扫 `pending` / `review` 且超过 `audit_timeout_seconds` 的图，转人工队列。
   三条通道的结论按 `block > review > pending > pass` 取最严，谁都不能把 `block` 翻回 `pass`。

5. **判定违规即删对象、并记违规**。`audit_auto_delete_block=true` 时删除七牛对象（删除是幂等的：对象已不存在按成功处理，七牛 612/631 与"no such file"文案都识别），同时经 `moderation` 端口写一条违规事件（图片 +20）。**管理员账号豁免**：`moderation.Service.Record` 对管理员返回 `Skipped: "管理员账号不记分"` —— 审核照审、图照删，但不给站长自己扣分（否则后台会把自己禁掉）。

6. **人工闸门在后台「审核 → 图片审核」**，两档队列：待处理（`pending` + `review`）与已驳回（`block`）；操作是放行 / 驳回 / 删除。**图片资源库在「媒体运营 → 图片资源库」**，只列 `pass`，支持关键词与时间范围筛选。两件事分开：库是"能用的图"，队列是"要人判断的图"。

7. **样式派发按尺寸档位，不按文件**。同一张图按四档样式名派发（列表 / 详情 / 头像 / 封面），样式名是可配置的字符串；`POST /images/style-probe` 用同一张图分别请求原图与四档样式并回状态码与字节数——样式名填错时七牛返回 404 而**不回退原图**，光看页面会以为是"图坏了"。

8. **`?imageInfo` 探测的硬约束**：七牛图片处理要求 fop 是**首个 query 参数**且**被签名覆盖**。写成 `?e=…&token=…&imageInfo`（先签名再追加）时七牛不报错，直接把**原图**当 200 返回；后端把 PNG 字节当 JSON 解，得到 `invalid character '\u0089' looking for beginning of value`（0x89 是 PNG 头），上传因此被判失败，而提示与真实原因毫无关系（2026-11-26 线上头像上传即此）。因此：签名必须用 `MakePrivateURLv2WithQueryString(mac, domain, key, "imageInfo", deadline)`。

9. **探测不能只押七牛那侧的可选能力**：`ImageInfo` 先走 fop，失败则**取回对象字节按文件头识别**（`image.DecodeConfig`，覆盖白名单里的 png / jpeg / gif / bmp）。兜底路径拿到的仍是真实字节，白名单与比例校验照旧跑在它上面，安全性不降；正常路径不多花一次下载。两条都失败时对外只回中文文案（`storage.image_info_invalid`，文案在 `apperr` 表里），fop 与字节两条通道各自的原因留在错误链里，由 `verify()` 打 warn 日志。

10. **审核状态是契约枚举，且必须与状态机同源**：`CreateTaskResponse` / `CompleteTaskResponse` / `TaskStatusResponse` 的 `audit_status` 都带 `enums:"pending,pass,review,block,deleted,orphaned,unknown"` 注解，两端 `openapi.d.ts` 因此生成字符串联合；前端按它列全映射表（`Record<ImageAuditStatus, …>`）而不是 `switch` + `default` —— 后端新增一种状态后，重新生成契约会让映射表**编译报错**，不会静默漏分支。`media/adapter/http` 有一条用例同时钉住"注解 = `domain.Status` 集合"与"三份 DTO 注解一致"。

11. **`status` 与 `audit_status` 是两件事，不许互相顶替**：`status` 是**任务**状态（`pending / ready / failed`，文件传完了没），`audit_status` 是**审核**状态（能不能给人看）。秒传命中的响应也走这条口径（任务即刻 `ready`，审核状态另给），否则客户端拿不到"为什么没有地址"。

12. **轮询必须回当前结论，不是入库快照**：任务在审核之前就落盘（`Status=ready`、`ImageURL` 快照），所以 `GetStatus` 必须按 `task.ImageID` 读图行的**当前**状态；只有图行已被物理清理时才回落到快照。照快照拼 `Image` 会让状态恒为 `0`（`unknown`），客户端于是把"已违规"报成"状态异常"（2026-11-27 线上）。

## 理由与已否决方案

- **否决「给图片审核单独一套七牛凭证」**：实测七牛只有一套 AK/SK（控制台无独立审核密钥），审核接口与上传接口共用同一对凭证；真正要改的是令牌前缀（AI 服务只认 `Qiniu`，SDK 默认签出 `QBox`）。
- **否决「把七牛回调当唯一真相」**：回调会不来、会慢（域名被封期、控制台漏配），而"没有结论"绝不能等价于"放行"。三条通道 + 超时转人工是"失败关闭"的实现。
- **否决「先给地址、判定违规后再回撤」**：CDN 已缓存，回撤只对后续请求有效；而且用户已经把图用在了页面上。不给地址（决策 3）比回撤便宜得多。
- **否决「关掉审核就恢复老行为也无所谓」**：保留 `audit_enabled` 开关是为了本地开发与故障回退，但**默认开**；关掉时新图直接 `pass` 入库，属于有意的降级，不是默认路径。
- **否决「审核状态用手写字符串联合」**：手写联合会在后端加状态时静默过期（`orphaned` / `unknown` 就这样漏过一版），而契约枚举 + `Record` 表让漏分支变成编译错误。
- **否决「用七牛 imageInfo 的尺寸当唯一真相」**：那是一个可选能力（依赖空间/域名配置与 fop 书写形态），坏了会把整条上传链拖下水；文件头兜底让"探测"不再是单点。

## 当前生效约束

- **上传入口**：读者站头像（`/studio/settings`）、文章封面与正文插图（`/studio/write`）、私信图（`/messages/chat`）；后台轮播封面（`/admin/media/banners`）、合集封面（`/admin/collections` 编辑浮层）。新增入口**必须**走 `POST /api/images/upload-tasks`。
- **配置**（`runtime_config.storage`）：`audit_enabled`、`audit_gate_enabled`、`audit_timeout_seconds`（默认 60）、`audit_auto_delete_block`、`whitelist`（默认 `bmp/jpg/jpeg/png`）、四档样式名与私有读、下载有效期。
- **状态短名是契约枚举**，与 `domain.Status` 同源；新增状态必须同时改 `domain`、swagger 注解与两端重新生成。
- **`block` 不可逆**：任何通道都不能把 `block` 改回 `pass`；要恢复只能重新上传（秒传会命中同一条已封禁记录，所以文案只说"换一张"）。
- **未过审的图不得出现在任何对外响应里**：`uploadVisibleURL` 是唯一出口，禁止绕过它直接拼 CDN 地址。
- **`?imageInfo` 必须首位且进签名**；`ImageInfo` 必须有字节兜底，且兜底结果同样过白名单与比例校验。
- **图片 GC 是账本口径，不扫桶**：它清的是"库里有行、但没有真实引用"的对象（`-image_gc` 清单 + `image_gc.enabled` 开关）。**桶里存在、库里没有任何行的对象不在它的覆盖范围**（要清只能去七牛控制台，或另做"桶 → 库"对账），这条缺口记在 [`docs/TODO.md`](../TODO.md) 里，不要在排查"图怎么还在"时把 GC 当成万能解。




