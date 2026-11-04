-- 0001_vectorstore.up.sql: chunk storage for the pgvector VectorStore adapter
-- (internal/adapter/vectorstore/pgvector).
--
-- This lives in its own migration directory (db/migrations_vectorstore/),
-- with its own independent golang-migrate version sequence starting at
-- 0001 — not numbered to follow db/migrations/'s sequence — because it is
-- meant to run against a *different* Postgres instance than the rest of the
-- schema (config.VectorStoreDatabaseURL). golang-migrate applies every file
-- under one -path to whatever single -database it's given, so mixing this
-- with db/migrations/ would silently run chunk-table DDL against the main
-- database too. Deliberately without foreign keys to knowledge_bases/
-- documents for the same cross-database reason — a foreign key across two
-- Postgres instances isn't possible, so consistency between chunks and the
-- knowledge_bases/documents rows they belong to is enforced in
-- usecase/knowledgebase.Service (it deletes a document's chunks explicitly
-- on re-ingestion or document deletion) rather than by the database. Run
-- via the golang-migrate CLI (see Makefile's migrate-vectorstore-up target)
-- against VECTORSTORE_DATABASE_URL if set, otherwise the same database as
-- db/migrations/0001_init.up.sql.

CREATE EXTENSION IF NOT EXISTS vector;

-- embedding dimension matches the embedder's output size (1536 for e.g.
-- OpenAI's text-embedding-3-small) — ASSUMPTION, adjust this column to
-- whichever embedding model a knowledge base is actually configured with
-- before ingesting real data. All knowledge bases in this deployment must
-- currently share one dimension, since it's fixed at the column level.
CREATE TABLE chunks (
    id                uuid PRIMARY KEY,
    document_id       uuid NOT NULL,
    knowledge_base_id uuid NOT NULL,
    chunk_index       int NOT NULL,
    content           text NOT NULL,
    embedding         vector(1536) NOT NULL
);

CREATE INDEX chunks_document_id_idx ON chunks (document_id);
CREATE INDEX chunks_kb_idx ON chunks (knowledge_base_id);
CREATE INDEX chunks_embedding_idx ON chunks USING hnsw (embedding vector_cosine_ops);
