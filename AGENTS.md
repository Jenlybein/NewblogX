# blogx

个人博客项目：Go 后端（`backend/`，module `myblogx`）+ 两个 Nuxt 前端（`frontend/` 读者站、`frontend_admin/` 管理后台）+ `deploy/` 基础设施。前后端唯一耦合是后端生成的 OpenAPI 契约。

- 运行 / 部署 / 运维：看 [`README.md`](./README.md)
- 文档入口：看 [`docs/README.md`](./docs/README.md)
- 领域词汇：看 [`GLOSSARY.md`](./GLOSSARY.md)

## Agent skills

### Issue tracker

Issues 在 GitHub Issues（`Jenlybein/NewblogX`），用 `gh` CLI 读写。见 `docs/agents/issue-tracker.md`。

### Triage labels

五个 triage 角色与标签字符串同名（`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`）。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `GLOSSARY.md` + `docs/adr/`。见 `docs/agents/domain.md`。
