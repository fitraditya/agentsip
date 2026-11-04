<div align="center">

<img src="docs/assets/dashboard.png" alt="AgentSIP dashboard" width="800" />

# 📞 AgentSIP

**A self-hosted voice AI engine that answers your SIP calls so you don't have to.**

Give it a SIP trunk, an STT provider, an LLM, a TTS voice, and (optionally) a knowledge base —
it handles the call, holds a real conversation, and hangs up when it's done.

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![Postgres](https://img.shields.io/badge/Postgres-pgvector-336791?logo=postgresql&logoColor=white)
![Status](https://img.shields.io/badge/status-personal%20project-lightgrey)

</div>

---

## What is this, actually?

AgentSIP is a lite, single-tenant fork of a larger multi-tenant voice-agent platform. No
org/workspace switching, no platform-admin API keys to juggle — just one deployment, one
admin login, and a dashboard for configuring everything a voice agent needs to pick up the
phone:

- 📠 **Trunks** — where calls come from, and the SIP auth guarding them
- 🎙️ **STT / 🧠 LLM / 🔊 TTS providers** — created once, reused across as many trunks as you
  like
- 📚 **Knowledge bases** — paste text or drop a PDF, it gets chunked, embedded, and made
  searchable by your agent mid-call
- 📋 **Call logs** — every call's disposition, duration, and outcome

If you've ever wanted your own Twilio-flavored AI receptionist without the "please contact
sales" pricing page, this is that, minus the SaaS.

## See it in action

<img src="docs/assets/trunk-form.png" alt="Creating a trunk" width="800" />

Every create/edit flow in the dashboard is a real page with a sidebar, not a modal you lose
your place in. Sections are cards with a title and a divider, because that's just nicer to
look at.

## How a call actually works

```mermaid
flowchart LR
    A[☎️ INVITE arrives] --> B{Trunk found<br/>and has room?}
    B -- no --> X[Refused: 403 / 503]
    B -- yes --> C{Answer mode}
    C -- auto --> D[Ring, then answer]
    C -- webhook --> E[POST to your webhook,<br/>wait for accept/reject]
    D --> F[🎙️ STT]
    E --> F
    F --> G[🧠 LLM<br/>+ optional knowledge-base lookup]
    G --> H[🔊 TTS]
    H -->|loop until BYE| F
    H --> I[📋 Call record saved]
```

No queues, no background workers — a call is handled start to finish in one Go process, with
Postgres as the only thing it talks to besides your chosen STT/LLM/TTS/embedding providers.

## Tech stack

| | |
|---|---|
| **Backend** | Go, [chi](https://github.com/go-chi/chi) router, [pgx](https://github.com/jackc/pgx) + Postgres, [sipgo](https://github.com/emiago/sipgo) for SIP, [pion](https://github.com/pion) for RTP/SDP, [pgvector](https://github.com/pgvector/pgvector) for embeddings |
| **Frontend** | React 19, Vite, TypeScript, Tailwind CSS v4, [Base UI](https://base-ui.com/), react-hook-form + zod, built on the [TailAdmin](https://tailadmin.com) template |
| **Voice providers** | ElevenLabs (STT/TTS), any OpenAI-compatible endpoint (LLM + embeddings) |
| **Document upload** | Drop a `.pdf`/`.txt`/`.md` straight into a knowledge base — text extraction and sanitization happen server-side |

## Quickstart

You'll need **Go 1.26+**, **Node 20+**, and a **Postgres** instance.

```bash
git clone <this-repo> agentsip && cd agentsip
```

### 1. Database

```bash
cd backend
cp .env.example .env
```

Edit `.env`: point `DATABASE_URL` at a Postgres database/role you've created (the example
default is `postgres://agentsip:password@127.0.0.1:5432/agentsip`), and fill in
`SEED_ADMIN_EMAIL`/`SEED_ADMIN_PASSWORD`. Then apply the schema (requires the
[`golang-migrate`](https://github.com/golang-migrate/migrate) CLI):

```bash
make migrate-up
```

`FIELD_ENCRYPTION_KEY` encrypts secrets (API keys, SIP passwords) at rest — generate one with:

```bash
openssl rand -base64 32
```

### 2. Run the engine

```bash
make run
# or: go run ./cmd/engine
```

This starts the admin HTTP API **and** the SIP/RTP listener in one process. On first boot, if
no users exist yet, it seeds exactly one admin account from `SEED_ADMIN_EMAIL`/`SEED_ADMIN_PASSWORD`.

### 3. Run the dashboard

```bash
cd ../frontend
npm install
npm run dev
```

Open `http://localhost:5173`, sign in with your seed-admin credentials, and start wiring up
providers → trunks → (optionally) a knowledge base.

## Project layout

```text
agentsip/
├── backend/
│   ├── cmd/engine/          # the one binary: admin API + SIP server
│   ├── internal/domain/     # entities, zero dependencies
│   ├── internal/usecase/    # business logic, one package per bounded context
│   ├── internal/adapter/    # HTTP, Postgres, SIP, provider clients — implements usecase ports
│   └── db/migrations/       # plain SQL, applied with golang-migrate
├── frontend/
│   ├── src/pages/main/      # one folder per feature, list + new + edit pages
│   ├── src/lib/api/         # typed client, one file per backend resource
│   └── src/components/      # shared UI (TailAdmin + Base UI)
└── .ai/docs/                # deep architecture docs — see below 👇
```

## Docs

This repo documents itself for both humans and AI coding agents:

- **[`.ai/docs/index.md`](.ai/docs/index.md)** — the real architecture docs: every domain
  (auth, trunks, providers, knowledge bases, the call engine) with sequence diagrams, schema
  tables, and `path:line` citations into the actual code. Start here if you want to understand
  *why* something works the way it does.
- **[`AGENTS.md`](AGENTS.md)** — instructions for AI coding agents working in this repo
  (build/test commands, architecture rules, things that'll bite you if you don't know them).
- **[`frontend/AGENTS.md`](frontend/AGENTS.md)** — frontend-specific conventions (routing,
  forms, i18n, the component patterns this dashboard follows).

## A few honest caveats

- Single-tenant by design — this is the *lite* fork. If you need multi-org support, you're
  looking for the bigger platform this came from.
- No async job queue yet — document ingestion and embedding happen synchronously on the
  request. Fine for a document here and there; a worker is the obvious next step for bulk
  imports.
- Call transcripts aren't persisted yet (the table exists, the writer doesn't) — see the
  [calls doc](.ai/docs/topics/calls/README.md) for the honest story.

None of these are secrets — they're written up in `.ai/docs/` in more detail than you probably
wanted.

## License

The frontend is built on [TailAdmin](https://tailadmin.com)'s free React template, MIT
licensed (see [`frontend/LICENSE.md`](frontend/LICENSE.md)). The rest of this project is a
personal/internal fork — check with the repo owner before treating it as open source in the
"go ahead and redistribute this" sense.

---

<div align="center">

Built with Go, React, and an unreasonable number of Playwright screenshots taken just to
double-check a dropdown rendered correctly.

</div>
