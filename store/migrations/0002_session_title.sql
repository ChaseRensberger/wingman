-- 0002_session_title.sql: add a human-facing title to sessions.
--
-- Sessions previously had only an opaque KSUID for identification. The
-- title is what UIs display in lists, headers, and rename dialogs.
-- Existing rows get an empty string default; the API/UI fall back to a
-- placeholder ("New session") when the field is empty so unmigrated
-- sessions don't render as blanks.

ALTER TABLE sessions ADD COLUMN title TEXT NOT NULL DEFAULT '';
