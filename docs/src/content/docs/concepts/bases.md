---
title: "Bases"
group: "Core"
order: 103
---

# Bases

A Base is a persisted directory workspace. It gives a client a named place to start and group sessions.

Each Base stores:

- A stable `bas_` ID.
- A display name.
- A filesystem path.
- The owning Wingman client.

The built-in default client always has a default Base named `Wingman`. If you omit `X-Wingman-Client`, `GET /bases` uses the built-in `Wingman` client and creates that default Base if it does not already exist.

## Create A Session In A Base

Create or reuse a Base, then create a session with `base_id`:

```bash
BASE_ID=$(curl -sS http://localhost:2323/bases | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"base_id\":\"${BASE_ID}\"}" | jq -r .id)
```

Wingman copies the Base path into the session's `work_dir` and records `base_id` on the session. Later Base path edits do not rewrite existing sessions.

Do not send both `working_directory` and `base_id` when creating or updating a session. Use `base_id` when the session belongs to a persisted workspace; use `working_directory` for an ad hoc directory.

## Web UI

The Sessions page is Base-first. It shows Base cards, opens a Base-specific session list when you click one, and keeps Base create/edit/delete controls on the Sessions page instead of a separate top-level Base page.
