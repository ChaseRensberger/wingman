---
title: "Logs and Diagnostics"
description: "Find server errors and inspect model calls."
---

# Logs and Diagnostics

Use server logs for HTTP and service failures. Use model-call diagnostics for provider errors, timing, and token usage.
A model call records one provider attempt. Retries have separate records with the same run and step.

## Inspect a model call

1. Open the affected session in the Console.
2. Open Inspector from the context-usage indicator.
3. Find the attempt under Model calls.
4. Select Copy diagnostics.

The snapshot includes request IDs, timing, usage, and failure details. It does not include the complete outbound request.
Review it before sharing. Wingman removes known credentials, but private conversation content can remain.

For the same records through the managed-service API, replace `ses_example`:

```bash
wingman api get /sessions/ses_example/model-calls |
  jq '.[] | select(.status == "failed") |
    {id, run_id, step, attempt, error_message, failure: .trace.failure}'
```

`wingman api` selects the local managed service. For a foreground or remote server, use the [direct HTTP setup](/concepts/authentication#direct-http-requests).

## Read logs

Wingman writes to standard error. The default format is JSON and the default level is `info`.
Model-call diagnostics do not require debug logging.

To save foreground debug logs, run:

```bash
wingman serve --log-format json --log-level debug 2>wingman-debug.jsonl
```

For recent logs from the managed service, run:

```bash
wingman api get /logs | jq -r '.[].raw'
```

The API retains up to 500 entries in memory. Restarting Wingman clears this buffer.
Managed services also keep a private file at `${XDG_STATE_HOME:-$HOME/.local/state}/wingman/wingman.log`.
At 50 MiB, Wingman reduces it to approximately 25 MiB of recent complete lines.
Foreground servers do not write this file.

To change managed-service logging, run `wingman service start --log-format json --log-level debug`.
Include any other runtime flags that you need.
Logs can contain private paths, query parameters, and error details. Model-diagnostic credential removal does not cover every log entry.

## Match requests to failures

| ID                    | Purpose                                   |
| --------------------- | ----------------------------------------- |
| `session_id`          | The conversation                          |
| `run_id`              | Work for one submitted message            |
| `model_call_id`       | One provider attempt                      |
| `step`, `attempt`     | The model step and its retry number       |
| `provider_request_id` | The provider's request record             |
| HTTP `X-Request-ID`   | The Wingman API request and its log entry |

The Wingman HTTP request ID differs from the provider request ID.
`trace.failure.request_id` contains the provider ID.
To find recent JSON logs for one attempt, replace `mcl_example`:

```bash
wingman api get /logs | jq '.[] | select(.attrs.model_call_id == "mcl_example")'
```

## Read failure details

`trace.failure` describes the failed provider request:

| Field                              | Meaning                                                               |
| ---------------------------------- | --------------------------------------------------------------------- |
| `stage`                            | `prepare`, `request`, `response`, or `stream`                         |
| `category`, `classification`       | Failure type and optional detail                                      |
| `http_status`                      | HTTP status when a response exists                                    |
| `code`, `type`, `message`, `param` | Provider error details                                                |
| `body`, `body_kind`                | Captured response, event, or stream prefix                            |
| `response_headers`, `causes`       | Response headers and underlying errors                                |
| `output_started`                   | Whether Wingman emitted text, reasoning, or tool input before failure |
| `transport`                        | Connection operation, delivery state, and recovery decision           |

HTTP 200 does not prove success. A provider can send an error after it opens the stream.
For example, exhausted credits can produce a `quota` failure with HTTP 200.
An incomplete stream body can contain earlier output and invalid JSON.

| Category                              | Meaning                                      |
| ------------------------------------- | -------------------------------------------- |
| `authentication`, `authorization`     | Invalid credentials or denied access         |
| `quota`                               | Exhausted credits or an account limit        |
| `rate_limit`                          | Temporary request throttling                 |
| `content_policy`                      | Provider policy rejection                    |
| `invalid_request`                     | A request that the provider rejects          |
| `unavailable`, `timeout`, `transport` | Provider availability or connection failure  |
| `decoding`                            | Invalid output or a missing completion event |
| `cancellation`                        | Canceled call                                |
| `provider`                            | Other provider failure                       |

Additional classifications include `context-overflow`, `payload-too-large`, and `incomplete-stream`.
HTTP status alone does not determine the category. HTTP 429 can indicate throttling or exhausted credits.

## Retries and timing

`trace.retry` records the decision, reason, and selected delay after a failed attempt.
`scheduled` means Wingman planned a retry. A later attempt record shows that it sent the request.
`retryable: true` does not guarantee another attempt. Attempt limits and cancellation still apply.

Normal execution does not retry a failure after a stream starts, even before visible output.
Restart recovery can send a replacement request and incur another charge. See [Run Status And Recovery](/concepts/sessions#run-status-and-recovery).

`trace.timing` records elapsed milliseconds for the first response, first activity, and first answer text.
Each retry starts a new clock. Missing fields mean that the activity did not occur or Wingman did not capture its time.
The inspector omits token rates when the required usage data is unavailable.

## Retention and limits

Saved sessions retain diagnostics across restarts. Ephemeral runs do not.
Failure bodies are limited to 64 KiB. Most text fields are limited to 2 KiB.
Headers have a 16 KiB total budget and a maximum of 64 names. Native error causes have count and depth limits.

`redacted` means that Wingman replaced captured values. `truncated` means that a size limit removed data.
`detail_omitted` means that some detail was unavailable.
An absent field does not prove that the event did not occur.

If Wingman is unavailable, run `wingman service status` and read its logs.
`GET /health` reports process health. Authenticated `GET /ready` reports server readiness, not provider access.
