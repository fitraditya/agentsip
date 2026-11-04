# AI & Engineering Doc Conventions

> Classification: INTERNAL

Rules every doc in `.ai/docs/` follows. Keep docs short.

## Frontmatter (6 fields, required)

```yaml
---
topic: <slug>
status: ready | stale
classification: PUBLIC | INTERNAL | CONFIDENTIAL
related: [<sibling-slug>, ...]
code: [<path>, <path>, ...]
verified_commit: <git sha> | no-git
---
```

- `status: stale` — anyone editing a file listed in `code:` without updating the spoke flips
  it to `stale`.
- `status: ready` — reviewer flips back after re-verifying against code, and bumps
  `verified_commit` to the SHA (or re-confirms `no-git`) they verified against.
- `classification` — default `INTERNAL`. Escalated to `CONFIDENTIAL` in this repo for: `auth`
  (token design, bcrypt/session internals), `providers` and `knowledge-base` (describe how
  API keys/DSNs are encrypted at rest), and `cross-cutting/data-flow` (the encrypted/hashed
  field map). Never generate a `RESTRICTED` doc — stop and ask instead.
- `code:` — primary repo-relative paths. Plain paths only (`path:line` goes in prose).
- `verified_commit:` — **this repo is not a git repository**, so every spoke uses the literal
  value `no-git`. Staleness here relies on manual `status` flips, not `git log` diffing — see
  `index.md`'s Limitations note. If this repo is ever put under git, switch new/re-verified
  spokes to a real short SHA and start diffing `verified_commit..HEAD` per CONVENTIONS as
  normal.

## Merge rule

Two candidate topics merge into one spoke when either:
- their `code:` path sets overlap ≥ ~40%, or
- you cannot describe one topic's sequence/state machine without re-explaining the other's.

Otherwise keep them separate, even if they share a user journey — use a hub-level lifecycle
diagram for that instead. Record every merge/split decision in `index.md`'s enumeration
table. Applied in this repo: `providers` merges STT+LLM+TTS (near-identical shape); `calls`
merges the low-level SIP/RTP engine with the call-record/decision/conversation business flow
(one state machine).

## Safety rule

Before writing any payload, example, or config value into a doc:
- Replace secret-shaped values (API keys, tokens, connection strings, passwords) with a
  placeholder. Never write a real value to any file.
- Mask PII-shaped values (NIK/KTP/KK/SIM/NIB/passport, phone, email, bank/card numbers) with
  a placeholder pattern.
- A live credential found during exploration is a finding to report to the user (with a
  rotation recommendation), not content for a doc. None were found while writing this doc set.

## Spoke template

Sections, in order — **skip any with nothing to say**:
TL;DR · Entry Points · Sequence · Schema · Key Components · Data · Failure & State · Gotchas ·
See Also.

Analysis topics (`structure`, `dependencies`, `request-flow`, `data-flow`, `api-surface`) use
the variant shapes seen in `topics/structure/README.md` etc. — same frontmatter and safety
rules, a body shaped for orientation rather than a single flow.

## Diagrams

- **Mermaid** only, inline in ` ```mermaid ` fences. No `.mmd` files, no `diagrams/` folder.
- Solid arrows = sync, dashed (`-.->`) = async (queue, background job, event). Note: this
  backend currently has **no async layer** (see `topics/cross-cutting/data-flow`) — dashed
  arrows won't appear in this doc set's own diagrams until one exists.

## Examples / payloads

- Inline ` ```json ` fences. No `examples/` folder.
- Sanitized per the safety rule above — always.

## Code references

- Prose: `path/to/file.ext:123`. Only cite a line you have opened.
- Frontmatter `code:`: plain paths only.

## Naming

- Topic folders kebab-case; cross-cutting under `topics/cross-cutting/`.
- One `README.md` per folder.
- Terms and service names exactly as in [GLOSSARY.md](GLOSSARY.md).
