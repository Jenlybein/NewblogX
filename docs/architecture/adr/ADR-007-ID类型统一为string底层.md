# ADR-007：ID 类型统一为 string 底层（类型即真相）

## Status

**Accepted / Implemented**（2026-10-26 拍板并当日落地）

> 前置：本 ADR 解决的是 ADR-003 遗留的"契约需要后处理才正确"问题。
> 拍板前的实测数据来自 `/tmp/be-probe`（backend 副本实验）；落地后的真实数据见文末"执行结果"。

## Context

### 现象：契约里 100 处 ID 被印成 `number`，而实际 JSON 是字符串

后端 `ctype.ID` 是这样定义的：

```go
type ID uint64                       // 存库是数字

func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String()) // 发出去是字符串
}
```

`swag` 读的是 **Go 类型声明**（`uint64`），它看不见 `MarshalJSON` 这一层。于是契约印 `integer`，实际传输是 `"372842048379686912"`。

### 为什么必须印 string：一道算术题

| | 值 | 位数 |
| --- | --- | --- |
| 现在生成的一个雪花 ID | 372,842,048,379,686,912 | **18 位** |
| JS 安全整数上限 2^53-1 | 9,007,199,254,740,991 | 16 位 |
| uint32 上限 | 4,294,967,295 | 10 位 |

**现有 ID 是 uint32 容量的 8680 万倍。** 反推：若强行塞进 uint32，时间戳只能用 1.023 秒就溢出。

顺带否掉一条"中间档"路线：只要 ID < 2^53，用 number 就安全——但保持现有位分配的话，时间戳只能用 **24.9 天**；且**存量 ID 已经全是 18 位**，改生成器救不了老数据，除非连外键一起迁移。

### 现状的三层补丁

```
swag init
   ↓  印错（integer）
backend/docs/swagger.json
   ↓  swagger2openapi（2.0 → 3.0）
   ↓  fix-openapi-id-types.mjs（138 处 integer → string）  ← 后处理补丁
   ↓  openapi-typescript
frontend/app/types/openapi.d.ts
```

补丁能工作，但它有两个结构性缺陷：

1. **只覆盖 `ctype.ID`**。实测加 `.swaggo`（`replace ...ctype.ID string`）后，schema 里 ID 字段的 integer 从 106 降到 45——剩 45 处是源码里**压根没用 `ctype.ID`、直接写的 `uint64`** 的字段，配置管不到。
2. **根因没消失**。"类型是数字、传输是字符串"这层隐式转换还在。以后谁新写一个 `uint64` ID 字段，契约会再错一次，而且这次没人会注意到。

### 附带发现：ID 类型有 10 个，不是 1 个

```
internal/kernel/ctype/id.go:15                    type ID         uint64
internal/article/domain/favorite/favorite.go:36   type ID         uint64
internal/auth/domain/session.go:14                type SessionID  uint64
internal/comment/domain/comment.go:9              type CommentID  uint64
internal/media/domain/image.go:12                 type ImageID    uint64
internal/user/domain/user.go:17                   type UserID     uint64
internal/article/domain/id.go:11,13,15,17         ArticleID / UserID / TagID / CategoryID（type 块）
```

10 套并存本身就是债：它们之间靠 `uint64` 强转互相转换，string 底层之后转换方式全变，正好趁这次理掉。

## 备选方案

### A. `.swaggo` 全局类型覆盖（已实测，可行但不彻底）

```
replace myblogx/internal/kernel/ctype.ID string
```

- 优点：1 行配置，5.6 秒生成，是 swag 官方支持的声明式机制（不是脚本）
- 缺点：只覆盖 `ctype.ID` 那 61 处；剩下 75 处（45 处裸 `uint64` 字段 + 约 30 处 `@Param` 注解）要另改；**隐式转换层还在**
- 判断：**有效，但买的是"这次对了"，不是"以后都对"**

### B. 逐字段 `swaggertype:"string"` 标注

- 优点：精确到字段
- 缺点：89 处标注，且**会让契约撒谎**——那些裸 `uint64` 字段发出的 JSON 真的就是数字，标成 string 等于骗前端
- 判断：**否决**。绕过问题的同时制造了新问题

### C. 维持生成后修补脚本（现状）

- 判断：**否决**。这正是"生成完再改一遍"，是本次要消灭的对象

### D. 换契约生成器（huma / go-fuego）

- 缺点：123 个接口全部重来
- 判断：**成本与收益不成比例**

### E. ID 类型底层改 string（推荐）

```go
type ID string                        // 类型即真相

func (id ID) Uint64() uint64 { ... }  // 需要数值时显式取
func (id ID) Value() (driver.Value, error)  // 入库仍转 int64
func (id *ID) Scan(src any) error           // 出库仍从 int64 转回
```

**已实测**（临时目录 `type ID string`），`swag` 直接输出：

```json
"id":      { "type": "string" },
"user_id": { "type": "string" },
"tag_ids": { "type": "array", "items": { "type": "string" } }
```

- **连 `.swaggo` 都不需要**，后处理脚本可删
- 数据库不动：`Value`/`Scan` 里照常转 int64，**零 DDL 约束不受影响**
- 消灭的是根因：隐式转换消失后，类型和 JSON 永远是同一个东西，swag 不可能印错
- 判断：**采用**

## Decision

**采用方案 E**，并把 10 个 ID 类型统一收敛。

### 统一策略

| 层 | 做法 |
| --- | --- |
| `ctype.ID` | 底层改 `string`，提供 `Uint64()` / `IsZero()` / `Uint64OrZero()` / `ParseID()` |
| 其余 9 个 domain ID 类型 | **全部改成 `ctype.ID` 的别名或同构 string 类型**，不再各自为政 |
| 裸 `uint64` 的 ID 字段（89 处） | 收编进 `ctype.ID`——这同时修掉了前端丢精度的潜在 bug |
| 非 ID 的整型字段（237 处：`Page`/`Limit`/`DiggCount`/`Size`/`Score`…） | **一个都不动**。它们值小、前端用 number 没问题、swag 印 integer 也是对的 |

### 为什么非 ID 的整型不跟着降 uint32

用户曾提议"ID 走 string，其余 uint64 降 uint32"。实测分类后否决：

- 所谓"其余 uint64"里，**89 处其实还是 ID**（`UserID`×20、`ArticleID`×7、`UploadID`×4…），降 uint32 就是那 8680 万倍的溢出
- 真正非 ID 的 237 处是分页/计数/尺寸，本来就用不到 32 位以上，降了没收益，反而给计数类字段埋 42 亿溢出的雷

### 执行前置（必须先清完）

本 ADR 的改动会触及约 240 处代码，**先把手上 3 项残留清完再开工**，避免两个大坑并行：

1. `useStudioArticle` / `useStudioAiAssist` 零测试
2. `InboxChatPanel` 图片 / 富文本发送协议
3. S2.8 字段级 swag 注解（`@Param` example/enum/required）

## 实测牵连面（副本实验，非估算）

方法：`cp -r backend /tmp/be-probe` → 改类型定义 → `go build -gcflags="-e" ./...` 迭代到收敛。

| 阶段 | 处数 | 内容 |
| --- | --- | --- |
| 10 个 ID 类型定义改 string | 40 | 含 `article/domain/id.go` 的 `type (...)` 块（单行 grep 会漏） |
| 业务代码主体 | 148 | `uint64(x)` → `x.Uint64()`、`x == 0` → `x == ""`、`return 0` → `return ""` |
| 迭代第二轮才暴露的 | 10 | 修完第一批才露出来 |
| 仍是 uint64 的 ID 字段 | 22 | `HomeStyleID`、`account.ID`、`row.UserID`、JWT claims… |
| 测试文件 | ≥21（低估） | `go vet` 只查到 3 个测试文件就停了 |
| **合计** | **≈240** | |

### 关键发现：192 → 22

只改 `ctype.ID` 时，残留 **192 处** `ctype.ID → domain.XxxID` 的转换错误。把 10 个类型统一之后，塌缩到 **22 处**。

这验证了"10 套类型并存本身就是债"：统一掉才是解药，剩下的 22 处是那些名字叫 ID、但压根没用 ID 类型的字段，本来就该收编。

## Consequences

### 变好的

- **契约一次生成就对**：删掉 `fix-openapi-id-types.mjs` 与 `.swaggo`，流水线从 4 步缩到 2 步
- **类型即真相**：不再有"类型是数字、传输是字符串"的隐式层，swag 不可能印错
- **新代码不会重犯**：谁新写 ID 字段，只要用 `ctype.ID`，契约自动正确
- **修掉潜在 bug**：89 处裸 `uint64` ID 字段现在真的在丢精度（前端），顺带治好
- **编译器强制审计**：`id == 0` 这类判断会全部编译报错，等于把全后端 ID 使用点强制过一遍

### 变差的 / 必须人工审的

**唯一不能靠编译器发现的风险：6 处 ID 大小比较会静默变语义。**

字符串比较下 `"10" < "9"` 成立，而数值比较下 `10 > 9`。位置：

| 文件 | 行 | 用途 | 后果 |
| --- | --- | --- | --- |
| `platform/es/articlesync/article_sync_load.go` | 615 / 618 | tag 排序的 tie-breaker（Sort 值相同时按 ID 排） | **不丢数据**，但排序结果变 |
| `article/apptest/repo_social.go` | 47 / 152 | fake 的 nextID 推进 | 同上 |
| `article/apptest/repo_social.go` | 112 / 271 | `sort.Slice` 按 ID 排 | 同上 |

> 更正一处先前的误判：615/618 **不是** ES 增量同步游标，只是 tie-breaker，所以不会漏数据。
> 但项目有 golden 测试（20 篇 × 12 接口逐字节比对），排序一变 golden 就会红——**不会静默漏过去**，代价是重新生成基线。

必须的处理：这 6 处改成显式数值比较（`a.ID.Uint64() < b.ID.Uint64()`），或确认为无害后保留并加注释。

### 其他成本

- 240 处改动 vs 方案 A 的约 119 处，**E 是 A 的两倍工作量**
- golden 基线需重新生成
- 涉及入库与解析，必须跑后端全量测试 + 两端 typecheck

## 配套守卫（必做，否则会重犯）

方案 A 的"以后不会再写错"是靠**约定**；E 之后要靠**机器**：

在 `backend/tools/archcheck` 增加规则 **禁止新增裸 `uint64` / `int64` 的 ID 语义字段**（字段名匹配 `^(id|.+_ids?|.+IDs?)$` 且类型为原生整型 → 违规）。

项目已有 archcheck 框架与棘轮机制（`--max-violations`），加一条规则成本很低，且与 E 的存量清零配合后可直接设基线 0。

## 执行结果（2026-10-26，真实落地数据）

验收全部可复跑：

```bash
cd backend && swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
# swagger.json：字段名匹配 ID 语义且 type=integer ⇒ 0 处
#              path/query/form 参数里 type=integer 的 ID ⇒ 0 处
cd backend && go build ./... && go vet ./...      # 干净，0 告警
# ⚠️ 必须再带 tag 跑一遍：go vet ./... 不编译 //go:build integration 的文件
cd backend && CGO_ENABLED=1 go vet -tags integration ./...
cd backend && CGO_ENABLED=1 go test ./... -count=1  # 82 包全绿
cd backend && go run ./tools/archcheck --skip-tests=false .  # 0 违规
test ! -f backend/.swaggo && test ! -f frontend/scripts/fix-openapi-id-types.mjs
```

### 预估 vs 实际

| 项 | 拍板时预估 | 实际 | 偏差原因 |
| --- | --- | --- | --- |
| 总改动 | ≈240 处 | **≈490 处**（45+ 文件） | 低估点全在下面三行 |
| 裸整型 ID 字段 | 22 处 | **175 处** | 原估算只扫了"看起来像 DTO"的文件；实际上 gorm row、domain 实体、port 输入结构、`kernel.IDRequest`、JWT claims 全都是裸 `uint64` |
| 测试文件 | ≥21（已标注低估） | **≈90 处** | `go vet` 才是准绳（它会编译 `_test.go`），`go build` 完全看不见 |
| 工厂/装配层 | 未列入 | **≈30 处** | `app/*_wiring.go` 在模块边界上做桥接，必然受影响 |
| 契约侧 ID 字段 | 100 处 | **41 处**（+32 个 `@Param`） | 方案的 `.swaggo` 覆盖此前已消掉一部分 |

**173 → 第 3 轮才收敛。** 编译器牵引是有效的，但要预期 3–4 轮：
第 1 轮是主体，第 2 轮是工厂与 port 签名，第 3 轮才是测试夹具。

### 26 个按理由豁免（不是遗漏）

| 范围 | 数量 | 理由 |
| --- | --- | --- |
| `internal/platform/log/**` | 6 | 对应 ClickHouse `UInt64` 列，外部真相在 `01-logs.sql`；改成 string 会与列类型不一致，且可能让 `JSONEachRow` 摄入失败 |
| `internal/testkit/schema/**` | 17 | 同上 |
| `ServerID`（conf ×3 + canalbase ×1） | 4 | MySQL 复制协议 `/ 雪花机器号`，uint32 节点编号，非业务 ID |
| `initializedID`（platform/idgen） | 1 | 雪花 worker 编号 |

这里体现一个原则：**"类型即真相"要求对齐的是各自的外部真相**——
HTTP 契约的真相是"JSON 里 ID 是字符串"，所以 DTO 用 `ctype.ID`；
日志表的真相是"CH 列是 UInt64"，所以落盘结构保持 `uint64`。
两边都对齐了，就不存在"两套说法"。

### 顺带修掉的隐性 bug

- `internal/chat/domain/chat.go` `BuildSessionID`：字符串比较下 `"10" < "9"` 成立，双方算出不同 `session_id` → **私信串号**。已改数值比较。
- `internal/site/domain/site.go` `Banner.CoverImageID`：人工审计脚本因行尾有注释漏掉，由新规则 R12 在上线当天抓出。
- `internal/auth/apptest/repos.go` 的 `fmt.Sscanf("access:%d:%d:%d", &c.UserID, ...)`：`%d` 扫进 string 会运行时报错，`vet` 不报（它只查 printf 系）。这类"格式串 + 自定义类型"的组合是本轮唯一需要逐处看的坑。

### 一个差点漏网的盲区：`//go:build integration`

**`go vet ./...` 默认不编译带 `//go:build integration` 的文件**，`go build` 也不编译测试。
于是第一轮"全绿"其实是假的——真跑 `bash scripts/check.sh` 时，
`go vet` 与 `go test -tags integration` 两个步骤同时红：

```
internal/site/app/service_test.go:127        rows[0].CoverImageID == 0
internal/chat/adapter/mysql/chat_integration_test.go   多处（行 83/98/100/149/172/206…）
internal/article/adapter/mysql/article_repo_integration_test.go  多处
internal/article/adapter/http/e2e_integration_test.go  domain.ArticleID(g.next()) 之类
internal/platform/log/writer/pipeline_integration_test.go
internal/platform/outboxrelay/relay_integration_test.go   %d 传 ctype.ID
```

共 **约 90 处**，全部集中在 integration 测试里。教训有三条：

1. **验证口径必须与门禁一致**：本地自测要跑 `check.sh`，或至少把 `-tags integration` 带上。
   只看 `go build` + `go vet` 会得到"全部通过"的错误信号。
2. **`go test -tags integration -run='^$' -gcflags=-e ./...` 是收集全量错误的最快方式**：
   `-run='^$'` 一个用例都不跑，只编译，`-gcflags=-e` 让编译器不因错误数过多而截断。
   `go vet` 分组输出且每包只报前几条，收敛极慢。
3. **`X(数值)` 形式在测试里比业务代码更常见**（`domain.ArticleID(g.next())`、`ID(900)`、
   `favorite.ID(i)`），而 `vet` 的 `stringintconv` 只在**不带 tag** 的那一轮看不到它们。

### 实际新增的守卫

`backend/tools/archcheck/rules_id.go` 的 **R12 `id-must-be-ctype`**：
字段名以 `ID` / `IDs` / `Id` / `Ids` 结尾，且类型是原生整型（含 `*` / `[]`）→ 违规。
7 条人造用例（4 条正例 + 3 条豁免反例），由 `TestAllRulesHaveSelfCheck` 元测试强制要求存在。
基线 0。

### 流水线：4 步 → 3 步 → 2 步

```
M7 之前：swag v1 → swagger2openapi → fix-openapi-id-types.mjs → openapi-typescript   （4 步）
M7 E5  ：swag v1 → swagger2openapi → openapi-typescript                              （3 步）
换 v2 后：swag v2 --v3.1 → openapi-typescript                                        （2 步）
```

## 追加决策（2026-10-26）：swag 换 v2，流水线压到 2 步

拍板后 E5-4 从"可选、单独排期"直接做了。

### 为什么原来卡在 3 步

`swag` 只有 **v1 会输出 Swagger 2.0**（实测：`{"swagger":"2.0"}`，顶层是
`definitions` / `securityDefinitions` / `basePath`），而 `openapi-typescript` 明确只吃
OpenAPI 3.x。两边都不肯让步，中间就必须放 `swagger2openapi` 当翻译。

v2 加了 `--v3.1` 开关，直出 OpenAPI 3.1，翻译层就不需要了。

### 切换前做的对比验证（不是直接替换）

用 v2 生成到临时目录，与"v1 + swagger2openapi"的产物逐 schema、逐字段比对：

| 项 | 结果 |
| --- | --- |
| schema 数量与名字 | 107 / 107，**完全一致** |
| path 数量与名字 | 94 / 94，**完全一致** |
| 字段集合差异 | **0 处** |
| 字段类型差异 | **5 处**（见下，已修） |
| `contract-guard.ts` 的 31 条断言 | **全过**（schema 名字没变，所以守卫不受影响） |

### 5 处类型放松，以及怎么修的

v2 遇到"字段类型是带枚举的 domain 类型 + tag 里又写了 `enums` 收窄"时，会同时输出
`$ref`（指向完整枚举）和 `enum`（收窄）。`openapi-typescript` 只认 `$ref`，
于是 `status: 1 | 2` 被放松成 `ArticleStatus`（= `1|2|3|4`）。

修法是给这 5 个字段加 `swaggertype`，让 v2 输出内联枚举而不是 `$ref`：

```go
Status ArticleStatus `json:"status" binding:"required,oneof=1 2" swaggertype:"integer" enums:"1,2"`
// 契约：{"enum":[1,2],"type":"integer"}   ← 精确，无 $ref
// 前端：status: 1 | 2
```

只用在 5 个字段上，不是全局方案——`swaggertype` 会丢掉"这是 `ArticleStatus` 类型"的语义，
所以只在该字段确实需要收窄时才用。

### 顺带删掉的东西

`docs.go` 和 `swagger.yaml` **无人消费**，一起删了：

- `docs.go`：gin 里没有挂 `/swagger` 路由，全仓无人 `import "myblogx/docs"`（已核实）
- `swagger.yaml`：没有任何脚本或工具读它

于是 `go.mod` 里那个 `github.com/swaggo/swag v1.16.6 // indirect` 也一并移除
（它只被 `docs.go` 引用）。`docs/` 现在只有一个产物：`swagger.json`。

### 剩余差异（可接受，都是无害的）

| 差异 | 影响 |
| --- | --- |
| `requestBody` 从 `components["requestBodies"]["X"]` 变内联 `{content:{...}}` | 语义相同，只是少一层间接 |
| 9 处 `requestBody?: never` → `{content: {"application/json": Record<string, never>}}` | 从"不允许传 body"变成"允许传空对象"，后端忽略，无实际影响 |
| 新增 `/** @enum {integer} */` 之类 JSDoc | IDE 提示更清楚 |

### 风险与回滚

`swag v2` 上游最新就是 `v2.0.0-rc6`，**没有正式版**。这是本次决策唯一实质性的风险。

回滚只需三步：`cp ~/go/bin/swag-v1.16.4.exe ~/go/bin/swag`，
Makefile 去掉 `--v3.1 --outputTypes json`，两端 `openapi:types` 加回 `swagger2openapi`。
v1 的二进制和原始产物都留了备份。

### golden 基线重生成的实际结果（人工 diff 结论）

`UPDATE_GOLDEN=1` 重生成后逐文件比对，**32 个 golden 里只有 1 个变**：

| 文件 | 变化 | 顺序是否变 |
| --- | --- | --- |
| `top_list.json` | `"list":[1019,1011,1003]` → `"list":["1019","1011","1003"]` | **否**，`1019 > 1011 > 1003` 完全一致 |
| 其余 31 个 | 逐字节未变 | — |

**为什么几乎没变**：`ctype.ID` 的 `MarshalJSON` 本来就输出字符串，所以 DTO 层的 JSON 形态
在改类型前后完全一样——变的只是"Go 类型"与"JSON 形态"是否一致这件事本身。

唯一变的是 `/articles/top` 的 `list`：它是**裸 `[]uint64`**，之前序列化成数字数组，
收编成 `[]ctype.ID` 后变成字符串数组。这是**修正**而不是回归：雪花 ID 作为 JS number 会丢精度。

顺手把该接口注解从 `list=[]object{}` 改成 `list=[]string`
（原来契约里是 `items: object`，前端拿到 `unknown[]`，等于没有类型）。

### 一个仍然存在的契约精度缺口

`handler_sub.go` 里还有 7 处 `list=[]object{}` 注解（文章列表、评论列表等）。
它们不都是 ID 数组，逐处确认哪个是 `[]string`、哪个该引用具体 DTO 才能印准。
这属于 S2.8 字段级注解的后续项，不在 M7 范围内。

## 验证

```bash
# 1. 契约一次生成就对（无后处理）
cd backend && rm -f .swaggo && swag init -g cmd/server/main.go ...
grep -c '"type": "integer"' docs/swagger.json | 与 ID 字段数对照  # 期望 0 处 ID 是 integer

# 2. 后端全量
go build ./... && go test ./...
cd backend/tools/archcheck && go test ./...

# 3. golden 重生成后必须再绿
# 4. 两端 typecheck + 前端契约守卫 contract-guard.ts
```

## 相关

- ADR-003：前后端契约以代码生成为单一来源（本 ADR 修的是它的第 4 条后果）
- `docs/architecture/10-M7决策清单.md`：任务分解与执行顺序
- `docs/architecture/baseline.md`：archcheck 棘轮基线
