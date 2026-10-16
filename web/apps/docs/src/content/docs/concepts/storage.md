---
title: "Storage"
description: "Choose a database and understand what Wingman saves."
---

# Storage

Wingman saves agents, conversations, runs, and provider credentials in a SQLite database.
The default path is `~/.local/share/wingman/wingman.db`.

To use another database, start the server with `--db`:

```bash
wingman serve --db /absolute/path/to/wingman.db
```

Saved runs retain their accepted input and configuration. Later agent edits or session moves do not change queued work.
Deleting a session permanently removes its history and related execution records.
See [Sessions](/concepts/sessions) for recovery behavior.

## Run without storage

Ephemeral mode does not save runs or conversations:

```bash
wingman serve --ephemeral
```

In this mode, use `POST /run` with an inline agent. Endpoints for stored resources return `501 Not Implemented`.
Supply provider keys through the server environment. See [Providers](/configure/providers#environment-variables).

## Conversation history

Wingman saves messages, model attempts, tool results, permission decisions, and session events.
Use the session API to read these records. Database tables are not a public API.

The standard server includes conversation compaction, which replaces older model context with a summary.
Earlier messages remain in the database and session history API. Compaction does not delete them.

## Database upgrades

Wingman applies database upgrades when it opens the store. It refuses to start if the recorded migration checksums do not match.

If Wingman reports incompatible migration history, do not change the database records to bypass the error.
Stop Wingman and back up the database with any associated `-wal` and `-shm` files.
To start with an empty database, set `--db` to a new path. The new database has no previous sessions, agents, or provider credentials.
