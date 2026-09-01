ALTER TABLE session_runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'message';
ALTER TABLE session_runs ADD COLUMN action TEXT NOT NULL DEFAULT '';
ALTER TABLE session_runs ADD COLUMN input_json TEXT;
