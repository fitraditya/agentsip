# AGENTS.md — AgentSIP Instructions

> Classification: INTERNAL
>
> Single source of truth for all AI agents (Claude, Cursor, Copilot) across this monorepo.
> Edit only this file for repo-wide instructions. The frontend has its own, more detailed
> `frontend/AGENTS.md` for frontend-specific conventions — this file doesn't duplicate it,
> it links to it. Other agent config files (`CLAUDE.md`) route here via `@AGENTS.md`.

## Before you start

This applies to every change, not just large features — adding, modifying, or deleting even
a single function, endpoint, column, or config value counts.

1. **Mandatory:** open `.ai/docs/index.md`, then load the topic(s) whose `code:` frontmatter
   lists the files you're about to touch, before writing any code. If no topic lists the
   file, check `.ai/docs/topics/structure` or `.ai/docs/topics/dependencies` for where it
   fits, and flag the gap — don't skip the lookup because nothing matched on the first try.
   If you're touching `frontend/src/*`, also read `frontend/AGENTS.md`. Use terms exactly as
   in `.ai/docs/GLOSSARY.md`.
2. Run the relevant Safe command below for the side of the repo you're changing.
3. If baseline is red, stop and report — do not build on a broken tree.

## Definition of done

- [ ] For backend changes: `cd backend && go build ./... && go vet ./... && gofmt -l .` and
      `go test ./...` all exit clean.
- [ ] For frontend changes: `cd frontend && npm run build && npm run lint` both exit clean.
- [ ] New/changed backend behavior has Go tests (this repo has a real Go test suite under
      `backend/`). The frontend has no test suite today — rely on `build`+`lint` plus manual
      verification for frontend changes.
- [ ] Affected `.ai/docs` topic(s) updated in this same change per "Doc updates are part of
      the task" below — `status` accurate (set to `stale` if you can't fully re-verify).
- [ ] No PII or secrets in code, logs, fixtures, or docs.
- [ ] Only files in the task's scope changed.

## Project overview

AgentSIP is a voice-agent SIP engine: a Go backend that terminates SIP calls, runs an
STT→LLM→TTS conversation loop per call, and exposes an admin REST API for configuring
trunks, STT/LLM/TTS providers, and knowledge bases; a React/TailAdmin frontend drives that
admin API. Single Postgres datastore (plus an optional separate bring-your-own Postgres for
vector chunks), no queue/async layer, single-tenant with a seed-admin bootstrap.

## Safe commands

```bash
# Backend (from backend/)
go build ./...
go vet ./...
gofmt -l .
go test ./...
make build   # -> bin/engine
make test    # go test ./...
make lint    # gofmt -l + go vet
make migrate-up / migrate-down                         # requires golang-migrate CLI + DATABASE_URL
make migrate-vectorstore-up / migrate-vectorstore-down  # requires VECTORSTORE_DATABASE_URL (or falls back to DATABASE_URL)

# Frontend (from frontend/)
npm run dev      # Vite dev server
npm run build    # tsc -b && vite build
npm run lint     # oxlint + eslint
npm run lint:fix
npm run format   # prettier
```

This repo is **not a git repository** — don't assume `git log`/`git diff` work from the
repo root; they won't.

## Architecture

| Layer | Folder | Responsibility | Hard constraint |
|---|---|---|---|
| Domain | `backend/internal/domain` | Pure entities, no framework/IO deps | Must import nothing internal |
| Usecase | `backend/internal/usecase/<name>` | Business logic, declares its own port interfaces | Imports `domain` only (convention, not enforced by tooling) |
| Adapter | `backend/internal/adapter/<name>` | Implements usecase ports: HTTP, Postgres, SIP, provider clients | Imports `usecase`+`domain`, never the reverse (convention only) |
| Composition root | `backend/cmd/engine` | Wires every layer together | The only place all layers are imported |
| API client | `frontend/src/lib/api` | Typed client per backend resource | Pages depend on it, not the reverse |
| Pages | `frontend/src/pages/main/<feature>` | Route-level screens | Routes defined only in `frontend/src/App.tsx` |

Full detail: [.ai/docs/topics/structure](.ai/docs/topics/structure/README.md). Frontend-only
conventions (dedicated-pages-not-modals, the `ComponentCard` pattern, the Base UI `Select`
`items`-prop gotcha, the four-locale i18n rule): `frontend/AGENTS.md` and
[.ai/docs/topics/cross-cutting/frontend-conventions](.ai/docs/topics/cross-cutting/frontend-conventions/README.md).

## AI Documentation

**Hub:** `.ai/docs/index.md` — start here. The Topic Index links every topic to its own
self-contained folder. Load only what your task needs; `structure` + `dependencies` for fast
orientation, a feature spoke for deep work.

**Reference:**
- `.ai/docs/CONVENTIONS.md` — doc template, frontmatter spec, diagram rules, merge rule,
  safety rule.
- `.ai/docs/GLOSSARY.md` — canonical terminology and external service names. Use the bolded
  form when writing.

## Doc updates are part of the task — not optional, not a follow-up PR

**A change is not done until this step runs.** Before you report a task finished, re-check
every file you touched (added, modified, *or deleted*) against every topic's `code:`
frontmatter in `.ai/docs/topics/`, and decide using this table:

| What changed | Action |
|---|---|
| Flow, contract, endpoint shape, status semantics | Update the topic's `README.md` in the **same change**, re-verify its claims, keep `status: ready` |
| New/renamed endpoint, table, status value, external service | Add the term to `GLOSSARY.md` first, then extend an existing topic or add a new one (apply the merge rule in `.ai/docs/CONVENTIONS.md`) and link it from `index.md`'s Topic Index |
| A file deleted or a whole feature removed | Remove the dangling `code:` entry (or the whole topic if nothing's left) and clean up any `related:`/link references to it elsewhere |
| Cosmetic / behavior-identical refactor | No doc change needed |
| Unsure | Set `status: stale` on the affected topic and say so — never leave it `ready` while unsure |

A file with no matching `code:` entry anywhere is itself a finding: either the enumeration
in `index.md` missed a module, or it's genuinely out of scope — say so, don't silently skip.

**This repo is not under git**, so `verified_commit` stays the literal string `no-git` — see
`.ai/docs/CONVENTIONS.md`'s frontmatter section. There is no `verify.py --staleness` check to
run here the way a git-tracked repo would have; rely on the manual `status` flip instead.

## How to add a new feature

1. Add entities in `backend/internal/domain`, if new ones are needed.
2. Add a migration under `backend/db/migrations` (or `migrations_vectorstore` for
   vector-store schema).
3. Implement the repository in `backend/internal/adapter/repository/postgres`.
4. Implement business logic in a new or existing `backend/internal/usecase/<name>`,
   depending only on `domain`.
5. Wire it in `backend/cmd/engine/main.go` (the composition root).
6. Add the HTTP handler in `backend/internal/adapter/http/handler`, register its routes in
   `backend/internal/adapter/http/router.go`.
7. On the frontend: add the resource to `frontend/src/lib/api`, then a dedicated routed page
   set under `frontend/src/pages/main/<feature>` (list + `/new` + `/:id/edit` — see the
   dedicated-page convention in `frontend/AGENTS.md`), registered in `frontend/src/App.tsx`.
8. Add Go tests for the usecase/handler layer. Update `.ai/docs` per the table above.

## Testing and CI

| What changed | Required test | Location |
|---|---|---|
| Backend usecase logic | Go unit test, table-driven where it fits | `*_test.go` next to the file under test |
| Backend HTTP handler | Go test exercising the handler | same package |
| `backend/internal/pkg/*` utility | Go unit test | same package (e.g. `textextract_test.go`) |
| Frontend change | No automated test suite exists — verify manually (build + lint + run the dev server) | — |

No CI config file was found in this repo during doc generation — if one exists elsewhere
(e.g. a hosted CI service's own dashboard config), it wasn't visible from the working tree.

## Things to watch out for

- **Trunk STT/LLM/TTS fields are resolved-at-read-time, not persisted** — never set
  `Trunk.STT`/`.LLM`/`.TTS` directly outside a repository's scan function.
  ([topics/trunks](.ai/docs/topics/trunks/README.md))
- **A `PUT` on a provider or trunk is full-replace, not merge** — a blank secret field clears
  the stored secret. ([topics/providers](.ai/docs/topics/providers/README.md))
- **A 201 response from document ingestion does not mean it succeeded** — always check
  `document.status`; `FAILED` is reported in the row, not as an HTTP error.
  ([topics/knowledge-base](.ai/docs/topics/knowledge-base/README.md))
- **`ConversationTurn` has no persistence path today** despite its doc comment implying one —
  don't assume call transcripts are queryable. ([topics/calls](.ai/docs/topics/calls/README.md))
- **Trunk-at-capacity maps to SIP 503, not 486** — deliberate, documented in three places in
  the code. ([topics/calls](.ai/docs/topics/calls/README.md))
- **Always pass `items` to a single-value Base UI `Select`** backed by an array of
  options/records, or it silently shows raw IDs/enum values instead of labels.
  ([topics/cross-cutting/frontend-conventions](.ai/docs/topics/cross-cutting/frontend-conventions/README.md))
- **This repo has no git history** — don't run `git log`/`git blame` expecting output, and
  don't rely on `verified_commit` diffing for doc staleness.

## Don't

- Don't maintain a second, competing doc set — everything lives under `.ai/docs/`.
- Don't fork a doc — update the existing spoke.
- Don't introduce a synonym for a term already in `.ai/docs/GLOSSARY.md`.
- Don't write a real token, credential, or PII value into any doc — placeholders only.
- Don't leave dangling references when removing code — grep `.ai/docs/` and clean up.
- Don't duplicate `frontend/AGENTS.md`'s content here — link to it instead.
