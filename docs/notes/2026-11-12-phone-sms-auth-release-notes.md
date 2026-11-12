# 2026-11-12 手机号 + 短信注册/登录（`phone-sms-auth`）发布说明与部署要点

- 批次：站点登录改造——**注册入口从邮箱换成手机号**，落地票 `phone-sms-auth`。
- 范围：`POST /users/phone/verify`（发短信验证码）、`POST /users/phone`（登录，首次验证即建档）、`POST /users/password/set`（给尚无密码的账号设首个密码，要求短信复验）；`user_models` 补 `phone` 列与唯一索引；新增第十段运行时配置 `sms`（阿里云 PNVS 短信认证）与后台「站点 → 短信」页；读者站登录弹窗改成「手机号登录 / 邮箱登录 / 密码登录」三 tab，**移除邮箱注册 tab**；后台「登录与发码」页新增手机号相关开关与限流。
- 后端走阿里云官方 Go SDK（`dypnsapi-20170525/v2`），不手写签名。**验证码由平台生成、平台核验**（`SendSmsVerifyCode` → `CheckSmsVerifyCode`）：官方 SDK 的发送接口没有"自定义验证码"字段，所以本站不做本地生成/本地校验。
- 出处链：[ADR-0020](./../adr/0020-zero-ddl-exception-and-generic-framing-table.md) 文末「后续修订」立项**补列通道**（本批是首个使用者，登记 `user_models.phone`）；[ADR-0029](./../adr/0029-config-section-registry-and-nine-named-sections.md) 文末「后续修订：九段 → 十段（sms）」；`GLOSSARY.md` 的「零 DDL」「运行时配置」两条词条同步为「只做加法 / 十段」；`docs/architecture/configuration.md` 与 `docs/architecture/overview.md` §六 A1 同批对齐。

## 零 DDL 口径的变化（部署前必读）

本批第一次真正用到**补列通道**，口径从「零列变更 + 允许新增表」改为「只做加法：允许新增表 + 允许补列」：

- **仍禁止**改名、改类型、删列——CDC（ES 文章投影与 image-ref）按列名解析 binlog，那几类会让同步静默错位。
- **补列**必须登记在 `backend/internal/platform/mysql/ddl/migrate.go` 的 `columnAdditions`，由 `-db` 幂等执行（先查 `information_schema` 再决定是否 `ALTER`）。
- 老库上 `schema.sql` 的 `CREATE TABLE IF NOT EXISTS` 是**空操作**，不跑 `-db` 就永远没有 `phone` 列；而业务测试建表走 testkit 的 `AutoMigrate` 会自动补列，本地全绿也发现不了。
- 漏跑的症状：登录 / 资料接口报 `Unknown column 'phone'`，其它功能照常。

## 部署要点

0. 备份数据库；把当前镜像打回滚标签。
1. 按 `README.md` §11.4 的顺序**串行**重建镜像（内存受限机器不要并行构建）。
2. `docker compose up -d` 起服务（含 `mysql` / `redis`），`docker compose ps` 等全部 healthy。
3. **用新镜像手动跑一次 `-db`**（`schema.sql` 是 `go:embed` 进二进制的，旧镜像里没有这一列）：

   ```bash
   docker compose exec blogx_server /app/server \
     -f config/settings.yaml -db
   ```

   它做两件事：建出登记的表的（幂等），以及按 `columnAdditions` 给既有表补列。`-db` 启动时会 `InitES`，ES 不可达会直接 `fatal` 退出——先确认 `es` 已经 healthy。

4. **后台「站点 → 短信」页填阿里云凭证**：AccessKey ID / AccessKey Secret / 接入点（默认 `dypnsapi.aliyuncs.com`）/ 短信签名 / 模板 Code / 调用超时。密钥落库前加密（`enc:v1:`），回显是占位符，**读不回旧值**，改就覆盖。
5. **后台「站点 → 登录与发码」页打开「手机号登录」**（`site.login.phone_login`，默认 `false`）。不开的话前端登录弹窗不会落在手机号 tab 上，`POST /users/phone*` 也会直接回「站点未启用手机号登录」。
6. 确认「邮箱登录」(`email_login`) 与「用户名密码登录」(`username_pwd_login`) 按需仍是开的。**「邮箱注册」(`email_register`) 默认关闭**——它只管注册；关掉它不影响邮箱登录，也不影响已登录用户「绑定邮箱」（绑定走发码接口，按用途单独放行）。
7. 验收（逐条都看，别只看容器 healthy）：
   - 路由确实挂上了：`POST /users/phone/verify` 与 `POST /users/phone` 是**匿名**端点，未注册的号码也得能发码建档，所以它们不该返回 1003；空请求体应回参数错误而不是 404。`POST /users/password/set` 在鉴权组里，匿名访问应回 `code: 1003`；
   - 未填凭证时点「发送验证码」，应报「短信发送失败」，不是 500；
   - 填好凭证后用一个真实手机号收码 → 登录 → 后台「用户管理」里出现新账号，昵称是 `138****8000` 形状（不是完整号码）；
   - 该账号在「设置 → 账号安全」里点「设置密码」，收码后设密码，再用「用户名 + 密码」登录成功；
   - 老账号（有密码）改密码仍走原路（验旧密码），不受影响。

## 契约与类型

- 三份产物同批重生成：`cd backend && make openapi` → `backend/docs/swagger.json`（`make` 里带 `decorate-deco-contract.mjs` 后处理，手跑 `swag` 会漏这一步并判 STALE）→ 两端 `pnpm openapi:types`。
- `scripts/contract-structures.json` 的 `minimumEntries` 重新登记为 **158 / 202 / 369**（rebase 到 #364 之后的原 153/197/363），`addedIn` 追加 `ticket: "phone-sms-auth"` 的 5 个 path / 6 个 schema。**零删除**：邮箱注册端点保留（只关开关），所以没有 `ENTRY_COUNT_DROPPED` 风险。
- 图形验证码修复只给三个既有请求体加了两个可选字段（`SendEmailRequest` / `SendSmsRequest` / `PwdLoginRequest` 的 `captcha_id` + `captcha_code`），**条目数不变**（仍是 158/202/369），但 schema 内容变了，所以三份产物仍需同批重生成——`check-contract-freshness.mjs` 比的是逐字一致，不是只看条数。

## 已知限制

1. **`CheckSmsVerifyCode` 是否需要 `SchemeName` 尚未验证**——需要真实阿里云账号联调。若核验一直返回失败而控制台显示已发送，先看这个参数。
2. **图形验证码补上了前端控件**（同批修复：此前邮件与短信两条发码通道、以及密码登录，在打开 `site.login.captcha` 后一律 400，因为前端从不提交 `captcha_id` / `captcha_code`）。现在四个入口（邮箱发码 / 短信发码 / 邮箱注册发码 / 密码登录）与后台设置页的两条发码（绑定邮箱、设密码复验）共用 `useCaptchaProof()` + `ImageCaptchaField.vue`：开关关着不渲染也不带字段；开着时点图刷新、每次提交后换一张（验证码一次性）。`GET /api/imagecaptcha` 拿图。
3. **没有"绑定手机号"入口**：`sms.KindBindPhone` 与对应的仓储方法已随该功能一起删掉（号位 2 保留不复用）。老账号要换绑手机号，目前只能由运维改库。
4. 包覆盖率仍低于架构总纲 D8 的 90%/85%：新增逻辑已有 20 条单测，但那是**包级**达标工程。
5. QQ 登录仍未接前端入口（`qq_login` 打开也无 UI），与本批无关。

## 回滚

- 代码可整体回滚。库上多出的 `phone` 列与 `uk_user_phone` 索引**不需要删**：旧镜像不读它，唯一索引允许任意多行 `NULL`，对旧版本无副作用。
- `runtime_config_models` 里多一行 `sms` 段对旧镜像同样无害：`decodeAll` 是按 `RegisteredSections()` 遍历的，库里没登记的行**根本不会被读到**，不需要删。
- 若已在后台打开 `phone_login` 并产生短信费用，回滚代码后记得把该开关关掉（它在新旧版本都存在于 `site.login` 里）。

## 验收记录（本机，2026-11-12）

- `go build ./...`、`go vet ./...`、`go test ./...`（exit 0）、`archcheck --skip-tests=false` 全过。
- 契约双闸门：`check-contract-freshness.mjs` 判 `OK`（158/202/369 不低于基线，137 项新增结构在场）；`test-contract-freshness.mjs` 8/8。
- 读者站 typecheck / lint / `vitest run`（2725 passed / 233 files）；后台 typecheck / lint / `vitest run`（816 passed / 69 files）。
- 新增 25 条单测：后端 20 条（`user/domain` 号码校验与打码 4、`user/app` 手机号建档 4、`auth/app` 登录与设密码 7、`platform/sms` 发送判定 5）+ 前端 5 条（图形验证码进请求体的形状）。
- **没有跑过真机短信**：阿里云账号与凭证不可用，`SendSmsVerifyCode` / `CheckSmsVerifyCode` 两条真实调用路径未经验证；`platform/sms` 的单测只覆盖不发请求的分支。
