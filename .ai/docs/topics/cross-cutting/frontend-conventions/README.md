---
topic: frontend-conventions
status: ready
classification: INTERNAL
related: [structure, auth]
code:
  - frontend/AGENTS.md
  - frontend/src/components/shared/component-card.tsx
  - frontend/src/components/ui/select.tsx
  - frontend/src/App.tsx
  - frontend/src/messages
verified_commit: no-git
---

# Frontend Conventions

## TL;DR

This repo's own feature pages follow a small set of established conventions layered on top
of the TailAdmin/Base UI template: every create/edit flow is a dedicated routed page (not a
modal) except one, every feature-page section uses a shared title+divider card component, a
real Base UI gotcha around `Select`'s `items` prop has already been found and fixed once, and
every new i18n namespace must land in all four locale files. `frontend/AGENTS.md` already
documents most of this in more detail — this spoke verifies it against current code and
records anything that's drifted, rather than duplicating it.

## Dedicated-page-not-modal convention

Confirmed. `frontend/src/App.tsx` registers list, `/new`, and `/:id/edit` as separate routes
for every CRUD resource, e.g. `/trunks`, `/trunks/new`, `/trunks/:id/edit`
(`App.tsx:36-38`), and the same pattern for `/stt-providers[...]` (`:50-53`) and its LLM/TTS
siblings. Each new/edit page is a routed component imported at the top of `App.tsx`, not a
dialog.

**One confirmed exception**: `frontend/src/pages/main/knowledge-bases/detail/ingest-document-dialog.tsx`
is a genuine `Dialog`-based flow (imports `Dialog`/`DialogContent`/`DialogFooter`/etc. from
`@/components/ui/dialog`, controlled `open`/`onOpenChange` props) — this is the one remaining
dialog in the app (document ingestion into an existing knowledge base), distinct from the
full-page flows for trunks/providers/knowledge-base creation itself. See
[../../knowledge-base/README.md](../../knowledge-base/README.md).

## ComponentCard: the title+divider pattern

`frontend/src/components/shared/component-card.tsx:10-43` renders a title block
(optional description) followed by a body wrapped in a top-bordered div
(`border-t border-gray-100`) applying `space-y-6 p-4 sm:p-6` — this is the visual
title-then-divider-then-content pattern used across feature pages, e.g.
`frontend/src/pages/main/trunks/trunk-form.tsx:286,326-328`
(`<ComponentCard title="Providers" description="...">`).

`frontend/AGENTS.md`'s "UI demo sections use ComponentCard" line undersells this — current
code shows it's also the standard wrapper for real feature-page sections (trunk form, and by
the same pattern, every other dedicated create/edit page), not just template demo pages. Not
a factual error, just narrower than actual usage — worth a small wording update in
`AGENTS.md` next time it's touched.

## The Base UI `Select` `items`-prop gotcha

`frontend/src/components/ui/select.tsx:6` wraps Base UI's `Select.Root`. `SelectValue`
(`:22-33`) renders whatever Base UI resolves as the selected item's label — but **Base UI
needs an `items` prop** (an array of `{value, label}`) to map a stored `value` back to a
label. Without it, `Select.Value` falls back to showing the raw stored value instead of its
label.

Confirmed-correct usages (all pass `items`):
- `frontend/src/pages/main/trunks/trunk-form.tsx:222-229` (answer-mode select)
- `trunk-form.tsx:342-349` (STT provider select: `items={sttProviders.map(p => ({value: p.id, label: p.name}))}`)
- Same pattern for LLM (`:386-389`) and TTS (`:427-430`) provider selects

The multi-select knowledge-bases field (`trunk-form.tsx:121-181`) does **not** use `items` —
it renders a custom children-function inside `SelectValue` that manually looks up each KB's
name by id, a different workaround for the same underlying gotcha (multi-value chips can't
rely on a single-item `items` lookup the same way).

No currently-broken (missing-`items`, single-value) usage remains in the repo — every
single-value `Select` checked now passes `items`. (This repo has no git history to cite the
prior broken state/fix commit from — see Limitations in `index.md`.)

## Four-locale i18n rule

Confirmed. All four files — `frontend/src/messages/{en,es,de,ar}.json` — carry identical
top-level namespace keys. Spot-checked `sttProviders`: all four have the same sub-key set
(`title`, `createButton`, `deleting`, `deleteConfirm`, `table`, `empty`, `errors`, `form`,
`new`, `edit`) with full parity.

## `App.tsx` as the single routing source of truth

Confirmed — a repo-wide grep for `<Route path` outside `frontend/src/App.tsx` returns zero
matches. All page imports are static top-of-file imports, consistent with `AGENTS.md`'s own
"static imports, no lazy loading" claim.

## `frontend/AGENTS.md` drift — one correction found

One claim is now **factually wrong**, found by a different scout than the one that checked
drift directly: `frontend/AGENTS.md`'s header line describes the app as *"Client-only SPA
with no backend, auth, or global store."* This is stale — the app has a real backend
(`backend/`), a real auth system (session tokens, password reset, profile self-service — see
[../../auth/README.md](../../auth/README.md)), and `AuthContext`/`AuthProvider`
(`frontend/src/contexts/auth-context.tsx`) as a global store. This line should be corrected
the next time `frontend/AGENTS.md` is edited — it predates the backend being wired up and was
never updated.

All other spot-checked claims (no `components/ui` barrel, dialog convention, routing
single-source-of-truth) matched current code with no drift found.

## Gotchas

- Always pass `items` to a single-value Base UI `Select` whose options come from an array
  (providers, trunks, etc.) — omitting it silently degrades to showing raw IDs/enum values
  instead of labels, with no type error to catch it.
- Don't trust `frontend/AGENTS.md`'s opening description line about "no backend, auth" — it's
  stale. Everything else in that file checked out as accurate.
- When adding a new feature-page section, reach for `ComponentCard` by default — it's the
  established pattern even outside the demo pages its own doc comment implies.

## See Also
- [../../structure/README.md](../../structure/README.md)
- [../../auth/README.md](../../auth/README.md)
- `frontend/AGENTS.md` — the frontend's own, more detailed conventions doc; this spoke
  verifies it, it doesn't replace it.
