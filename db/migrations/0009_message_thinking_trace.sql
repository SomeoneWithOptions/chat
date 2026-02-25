-- Persist per-message progress traces for unified thinking visibility UX.
ALTER TABLE messages ADD COLUMN thinking_trace_json TEXT;
