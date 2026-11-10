# `skills/` —— 产品运行时策略文本

这个目录放的是**产品运行时**读给模型看的策略文本，**不是**给编码 agent 用的 skill。

| | 这里（`skills/`） | `docs/agents/`（AGENTS.md 那一套） |
| --- | --- | --- |
| 谁读 | 后端进程，启动时读盘 | 编码 agent（Claude Code / Codex 等） |
| 内容 | 捏捏的口吻、投稿机审的判定规则 | 本仓库怎么建 issue、怎么写 ADR |
| 改了影响 | 线上用户看到的话术与判定口径 | 只有 agent 的工作方式 |
| 位置决定 | 我们自己的选择（ADR-0033） | 各 agent 工具的约定 |

**格式**遵循 [Agent Skills 开放规范](https://agentskills.io/specification)，所以同一份
文件脱离本项目也能被任何兼容客户端直接使用；**位置**是本项目自己的选择——目前只有
本项目的进程消费它们，没有必要藏进点号目录。

## 布局

```
skills/
├── README.md          # 本文件（不是 skill，加载器会跳过根下的普通文件）
├── jelly-voice/
│   └── SKILL.md       # 读者站「捏捏」：口吻、称呼、禁用语
├── moderation/
│   └── SKILL.md       # 投稿机审：判定规则与注意事项
└── scoring/
    └── SKILL.md       # 文章质量评分：什么算值得发布、维度判据、反空洞规则
```

新增一份 skill 只需要加一个目录，**不用改代码**：

```bash
mkdir skills/<name>
$EDITOR skills/<name>/SKILL.md      # name 必须等于 <name>
```

## SKILL.md 的硬性约束

加载器与门禁（`scripts/check-skills.mjs`）逐条检查，任何一条不过都**启动失败**：

| 字段 | 约束 |
| --- | --- |
| `name` | 必填；小写字母 / 数字 / 连字符，不以连字符开头或结尾；≤64 字符；**必须等于父目录名** |
| `description` | 必填；1–1024 字符；写「什么时候用这份 skill」，触发全靠它 |
| `license` / `compatibility` / `allowed-tools` | 可选；`compatibility` ≤500 字符，`allowed-tools` 是空格分隔的工具名 |
| `metadata` | 可选；**只能是 `map[string]string`**。产品私有信息只写这里，不自造顶层字段 |

正文（frontmatter 之后）不能为空。规范会加字段，因此**未知顶层字段不会让加载失败**，
但「私有信息进 `metadata`」这条写作约定由评审与门禁的负向用例一起守。

## 进程怎么读它

- 进程启动时按 `<root>/*/SKILL.md` 扫描、只解析 frontmatter，把 **name + description**
  装进目录层；**正文按需读取**（`Skill.Body()`），不驻留在启动内存里。
- 目录来自进程级配置 `skills.dirs`（`BLOGX_SKILLS_DIRS`，逗号分隔），默认 `skills`。
  容器里镜像把本目录拷到 `/app/skills`；本地从 `backend/` 启动时用
  `BLOGX_SKILLS_DIRS='../skills'`（见 `.envrc.example`）。
- 目录读不到、或某份文件不合规 → **进程不启动**，错误里点名具体文件。没有代码内兜底
  文本：宁可起不来，也不要在线上用一段没人审过的旧话术回答读者。
- 「哪个场景用哪份 skill」是**产品接线**，写在运行期配置里（`ai.scene_skills`），
  不进 skill 本体——skill 要能被任何人拿走直接用。

## 本地校验

```bash
node scripts/check-skills.mjs              # 校验本目录
node scripts/check-skills.mjs --self-test  # 证明它对人造违规用例真的会红
```

门禁登记在 `scripts/gates/gates.json`（`skills-format`）；改完文案先跑这两条，
再跑 `go test ./internal/platform/skills/...`。

决定与理由见 [ADR-0033](../docs/adr/0033-runtime-policy-skills.md)。
