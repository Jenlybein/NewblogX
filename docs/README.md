# 文档地图

本仓库文档按「agent 配置 → 架构/规则 → 决策 → 笔记 → 归档」组织。正文中文；文件名用 ASCII kebab-case（`docs/agents/` 与 `docs/adr/` 由工程技能约定固定）。

## 目录

| 路径 | 内容 | 什么时候看 |
| --- | --- | --- |
| [`agents/`](./agents/) | 工程技能配置：issue tracker、triage 标签、domain docs 消费规则 | 换 issue 工具 / 改标签命名 / 接新技能时 |
| [`adr/`](./adr/) | 9 条架构决策（ADR-0001–0009） | **改规则或选型前先读**；冲突要显式指出 |
| [`architecture/`](./architecture/) | 现行架构与治理：overview / governance / contracts / configuration / ai-integration | 改代码前 |
| [`notes/`](./notes/) | 带日期的过程记录与复盘；`notes/memory/` 是 agent 长期记忆 | 复盘同类故障、查项目上下文 |
| [`archive/v1/`](./archive/v1/) | v1 归档原文（ADR 合集、基线与过程） | 只查历史，不再更新 |
| [`TODO.md`](./TODO.md) | 待办与想法池（未排期） | 选题 / 排期 |
| [根 `GLOSSARY.md`](../GLOSSARY.md) | 领域词汇表 | 命名、写 issue / ADR 前 |
| [根 `AGENTS.md`](../AGENTS.md) | agent 入口与技能配置摘要 | — |
| [根 `README.md`](../README.md) | 项目运行/部署/运维手册 | 起服务、部署 |

## 分工规则

- **决策** → `adr/`：一条 ADR 一个文件，编号递增，不覆盖旧决策（要推翻就写新 ADR 并在旧的 `状态` 里标 superseded）。
- **现行规则与设计** → `architecture/`：只写"现在是什么、必须遵守什么"；过程与数字留给 notes / archive。
- **过程** → `notes/YYYY-MM-DD-<slug>.md`：复盘、上线记录、排查经过，回答"当时发生了什么"。
- **历史** → `archive/`：只进不改（除链接修正）。
- **术语** → 根 `GLOSSARY.md`：概念释义；与 ADR 的分工是"词条=概念，ADR=决策"。
- **待办** → `TODO.md`：未排期的想法；排期后进 issue tracker（见 `agents/issue-tracker.md`）。

## 维护规则

- 文档链接一律用相对路径（仓库内可点）；重命名文件时同步 `grep` 全仓引用（含代码注释里的 `docs/...`）。
- 新增文档前先想清楚它回答哪一类问题，放进对应目录，不要都堆在 `docs/` 根。
- 中文正文 + 英文文件名；标题不超过一行；每篇开头写"这份文档是什么 / 什么时候翻它"。
