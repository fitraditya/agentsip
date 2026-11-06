-- 0002_add_greeting_fields: adds configurable greeting text and delay after greeting before STT starts.
ALTER TABLE trunks ADD COLUMN greeting_text TEXT NOT NULL DEFAULT '';
ALTER TABLE trunks ADD COLUMN greeting_delay_seconds INT NOT NULL DEFAULT 0;
