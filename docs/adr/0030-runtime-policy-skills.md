# ADR-0030：运行时策略 skill 放在仓库根 `skills/`，读不到就不启动

- 状态：已交付
- 日期：2026-11-08
- 来源：parent spec [#302](https://github.com/Jenlybein/NewblogX/issues/302)（发布治理一期）· 落地票 [#309](https://github.com/Jenlybein/NewblogX/issues/309)（策略文本加载地基）· 决定过程见 [grill 记录 §五](../notes/2026-11-07-publish-gate-and-moderation-grill.md#五skills-与通用-skill-加载器)

## 背景与问题

捏捏的口吻与投稿机审的判定规则，原本只能写在 Go 的 system prompt 常量里：改一句话要走代码、评审、发版，而改错的代价由读者与作者先发现（#302 问题陈述）。运营要的是「不发版就能改文案」，同时要求「改坏了别让用户发现」。

两件事把形状定死了：

- **格式**要可移植：文本遵循 [Agent Skills 开放规范](https://agentskills.io/specification)（`name` + `description` 必填，可选 `license` / `compatibility` / `metadata` / `allowed-tools`；`name` 必须等于父目录名；`metadata` 只能是 `map[string]string`），别人拿走就能直接用，将来也能被别的工具消费。
- **位置**是我们自己的：当前只有本项目的进程读这两份文件，没有第三方客户端要读；把目录藏进 `.agents/skills` 只会多一层解释成本（`.agents/` 是编码 agent 的约定，不是产品运行时的约定）。

还有一个硬约束：`go:embed` **到不了父目录**。`skills/` 在仓库根、Go 模块在 `backend/`，所以这只能是**启动时读盘**——不是疏忽，是决定的形状。

`skills/` 同时又落在所有现有门禁的盲区里：archcheck 只扫 `backend/**/*.go`，前端 lint 只看两个 app。不补一道自己的门禁，策略文本就是「永远绿的暗礁」。

## 决策

1. **目录放仓库根 `skills/`**，当前两份真 skill：`jelly-voice/`（捏捏的口吻、称呼、禁用语）与 `moderation/`（投稿机审的判定规则与注意事项），外加一份 `README.md` 说明「这是产品运行时策略，不是编码 agent 的 skill」。`README.md` 是根下的普通文件，加载器跳过它。
2. **加载器进 `backend/internal/platform/skills`**：只依赖标准库 + 仓库已有的 `gopkg.in/yaml.v3`，**不 import 任何业务包**。`internal/platform/...` 是 archcheck 的共享设施豁免位；放进 `internal/<业务模块>/` 会被当成业务模块受 R10/R11 约束。「通用」在本项目里指**格式合规 + 实现无业务耦合**，不指对外发布成可 import 的库。
3. **渐进披露分两层**：`Load(roots)` 扫 `<root>/*/SKILL.md`、只解析 frontmatter，目录层只有 **name + description**；正文留到 `Skill.Body()` 才读盘。版本号 `Skill.Hash()` = SKILL.md 字节的 sha256 前 12 位，可随调用记录落库（接线在后续票）。
4. **目录来自进程级配置**：`conf.Skills.Dirs` ← `BLOGX_SKILLS_DIRS`（逗号分隔，相对**进程工作目录**解析），默认 `skills`，默认值只写在 `conf.DefaultSkillsDirs` 一处（`settings.yaml` 里是空占位符，不抄第二份默认值）。
5. **读不到就不启动**：`app.InitInfra` 在连数据库**之前**调 `app.LoadSkills`；目录缺失、名称与目录名不一致、描述超长、正文为空、`metadata` 值不是字符串，一律返回点名具体文件的错误，由 `main` 变成进程起不来。**没有代码内兜底文本。**
6. **部署侧把目录带进镜像**：`deploy/docker/api/Dockerfile` 的 `COPY skills /app/skills`（WORKDIR 是 `/app`，与默认 `skills` 对齐）；要改文案不发版，把宿主机目录挂到 `/app/skills` 覆盖即可。本地从 `backend/` 起进程，所以 `.envrc.example` 给 `BLOGX_SKILLS_DIRS='../skills'`。
7. **补一道门禁**：`scripts/check-skills.mjs` 用等价规则校验（`--root` 可指定目录），`--self-test` 用 5 个人造违规用例 + 2 个对照用例证明它真的会红；登记在 `scripts/gates/gates.json` 的 `skills-format`（group `backend`，`steps` 由 runner 直接执行，因此本地 `check.sh` 与 CI 都自动跑）。**演练节号与 `archcheck` 共用第 3 节**：这道门禁的负向用例写在它自己的 `--self-test` 里，与第 3 节「规则自检」是同一种证明形状；`verify-gates.sh` 的 `1..14` 编号与 `/14` 标记因此一字未动。
8. **零 DDL**：这一步不碰数据库，没有新表、没有列变更。

## 理由与已否决方案

- **否决 `go:embed`**：到不了父目录（要么把文本塞进 `backend/` 破坏「仓库根」这个决定，要么在构建时复制一份产生两个事实源）；而且 embed 会把文案钉在二进制里，「不发版改文案」直接落空。
- **否决放进数据库运行时配置**：它影响的是「进程怎么跑」，不是某次请求的行为。放进能热改的那一层，就会出现「进程起来了、策略文本却没了」的中间态；而这类故障的表现是回答变得莫名其妙，不是报错。
- **否决代码内兜底文本**（哪怕只是一句「服务暂时不可用」）：兜底文本没人审、不会随版本更新，而且会把「启动失败」这个明确信号换成「用户先发现」。
- **否决对自家 skill 走宽松模式**：规范建议对第三方 skill 目录「跳过并 warn」，但随仓库走的两份是**我们的产品文本**，跳过等于静默用半套策略跑线上。宽松模式留给将来真正接入外部 root 时再说。
- **否决把校验塞进 Go 门禁或 archcheck**：改文案的人不一定装 Go；archcheck 的输入是 `backend/**/*.go`，它看不见根目录的 Markdown。
- **否决依赖官方参考校验器 `skills-ref`**：它不是本仓库的依赖，装不上就会让门禁变成「看环境脸色」。等价规则写进脚本（规则表与加载器逐条对齐），负向用例跟着脚本走。
- **否决把「哪个场景用哪份 skill」写进 `metadata`**：`metadata` 是**描述**，真相在运行期配置（`ai.scene_skills`），因为要能临时摘掉某份 skill 而不发版。

## 代价 / 不做什么

- `skills/**` 对 archcheck 完全不可见，这道门禁是它唯一的结构性守卫；规则在 Go 与 Node 各有一份实现，改规则必须同时改两处（两边都有负向用例，漏改会红）。
- 启动强依赖磁盘：镜像忘了带 `skills/`、本地 `.envrc` 少了 `BLOGX_SKILLS_DIRS`，进程都**起不来**。这是有意的失败面——把「文案丢了」从运行期挪到部署期。
- 本次**不接消费点**：捏捏的 system prompt 与机审的提示词拼接是后续票（#302 的分票），`Infra.Skills` 现在只被加载与校验，没有调用方。
- 不做「未知场景」校验：场景绑定还没接线（运行期配置 `ai.scene_skills` 尚未落地），等接线时在配置校验里做，不在文本里做。
- 不做 `references/` / `scripts/` / `assets/` 等可选子目录的资源接口（`Resource` / `ListResources`），当前两份 skill 不需要；需要时按规范补，不影响已有 API。
- 零 DDL；不改任何既有表、端点与契约。

## 当前生效约束

1. `skills/<name>/SKILL.md` 只用规范字段：`name`（必填，≤64，小写字母/数字/连字符，等于父目录名）、`description`（必填，1–1024，写「什么时候用它」）、`license` / `compatibility`（≤500）/ `allowed-tools`（空格分隔）、`metadata`（**只能 `map[string]string`**）。**未知顶层字段容忍**（规范会加字段），私有信息只进 `metadata`。
2. 正文（frontmatter 之后）非空。启动只读目录层，正文按需读取，不在启动时驻留。
3. `Hash()` = SKILL.md 字节 sha256 前 12 位，用于回答「线上跑的是哪一版策略文本」。
4. 目录默认值只有 `conf.DefaultSkillsDirs` 一处；`settings.yaml` 写 `${BLOGX_SKILLS_DIRS:-}`，不出现第二个字面量。
5. 加载失败必须让进程起不来，错误里必须点名文件或目录；不得改成「warn 后继续」，不得引入兜底文本。
6. 新增一份 skill 只加一个目录，不改代码；改了文案先跑 `node scripts/check-skills.mjs`（与启动校验同一套规则）。
7. 零 DDL 口径不变。

## 相关代码与文档

- 概念：[GLOSSARY · 运行时策略 skill](../../GLOSSARY.md) · [grill §五](../notes/2026-11-07-publish-gate-and-moderation-grill.md#五skills-与通用-skill-加载器) · [Agent Skills 规范](https://agentskills.io/specification)
- 加载器：`backend/internal/platform/skills/{skills.go,frontmatter.go,skills_test.go}`
- 启动接线：`backend/app/{skills_wiring.go,skills_wiring_test.go,bootstrap.go}`、`backend/conf/{conf_skills.go,conf_skills_test.go,enter.go}`、`backend/config/settings.yaml`
- 策略文本：`skills/README.md`、`skills/jelly-voice/SKILL.md`、`skills/moderation/SKILL.md`
- 门禁：`scripts/check-skills.mjs`、`scripts/gates/gates.json`（`skills-format`）、`scripts/gates/test-gate-runner.mjs`
- 部署：`deploy/docker/api/Dockerfile`、`.envrc.example`、根 `README.md` §7.3
