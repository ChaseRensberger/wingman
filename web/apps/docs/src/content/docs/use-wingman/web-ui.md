---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

The Console is Wingman's browser interface. Use it to manage agents, select providers, and send session messages.
It uses the same server address as the API.

## Local Console

After you start Wingman, open this URL:

```text
http://localhost:2424/console/
```

Enter the credentials from `~/.config/wingman/service.env` in the browser
prompt. The managed service uses these credentials.
The foreground server uses them unless it has explicit environment credentials.

To open the local managed service's Console:

```bash
wingman console
```

To open a new session in the current directory, run `wingman console .`.
Wingman reuses the workspace for that directory or creates one if needed.
The Console saves the session when you send its first message.

After a server disconnect, the Console waits for the server and reloads the current page when it returns.

## Model-call Diagnostics

Open Inspector from the context-usage indicator. Select Inspect model input for an attempt under Model calls to read its recorded content.
The view includes the system prompt, messages, tool definitions, and instruction sources.
Select Copy diagnostics for errors, timing, usage, and request IDs.
Read [Observability](/use-wingman/observability) for diagnostic fields, logs, and capture limits.

## Answer questions

Pending questions appear above the message composer. Select a suggested answer or type your own answer.
Some prompts accept multiple selections. For a request with several prompts, use Next and Back to move between them.
Select Submit answers after you answer every prompt.

Dismiss tells the model that you declined to answer. The run can continue.
Stop run cancels the run and its pending questions.
A browser disconnect or server restart does not discard saved questions or submitted answers.
See [Questions](/concepts/sessions#questions) for details.

## Compaction

Compaction replaces older model context with a summary. It does not delete the saved conversation.
The Console shows a Compaction row with a progress indicator while the operation runs.
After completion, the row shows input and output token usage when the provider reports it.

Expand the row to read its summary when one is available.
Inspect a later model call to see the summary and recent context prepared for that call.
If the operation fails, the row shows Compaction failed.

## Macros

Type `/` in a Session composer to show project macros. Type an argument after
the macro ID, then send the message. See [Macros](/configure/macros) to create
project macros.
