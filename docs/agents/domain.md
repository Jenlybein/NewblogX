# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`GLOSSARY.md`** at the repo root: the project's domain vocabulary. Use these terms verbatim in issue titles, refactor proposals, hypotheses and test names.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in before changing a rule or a selection.
- Background, only when an ADR or the task points at it:
  - `docs/architecture/` — current design and rules (`overview.md` §六 is the hard-constraints table; `governance.md`, `contracts.md`, `configuration.md`, `ai-integration.md` for专项).
  - `docs/notes/` — dated postmortems and working notes; `docs/notes/memory/MEMORY.md` is the pinned long-term memory (facts/约定), the per-day files are historical logs and are not rewritten.
  - `docs/archive/v1/` — v1 archive; historical value only.

If a file above doesn't exist, **proceed silently**. Don't flag its absence and don't suggest creating it upfront. `/domain-modeling` (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates glossary terms and ADRs lazily when terms or decisions actually get resolved.

## File structure

Single-context repo:

```
/
├── AGENTS.md                 ← agent entry point
├── GLOSSARY.md               ← domain vocabulary
├── docs/
│   ├── agents/               ← this config (issue tracker / triage labels / domain docs)
│   ├── adr/                  ← 0001-…-0011-…, one decision per file
│   ├── architecture/         ← current design + rules
│   ├── notes/                ← dated notes, memory/
│   ├── archive/v1/           ← historical v1 archive
│   └── TODO.md
├── backend/
├── frontend/
└── frontend_admin/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `GLOSSARY.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (ID 类型统一为 string 底层), but worth reopening because…_
