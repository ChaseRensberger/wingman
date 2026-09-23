---
title: "Observability"
description: "Use daemon logs and model-call diagnostics to investigate failed runs, provider errors, and connection failures."
---

# Observability

Wingman provides daemon logs, health endpoints, and persistent records of model calls.
The Console inspector can copy detailed evidence for an individual model call.
Normal chat errors remain concise.

## Choose the Right Record

| Question                                | Record                                                                |
| --------------------------------------- | --------------------------------------------------------------------- |
| Did the server start?                   | `wingman service status`, `/health`, and `/ready`                     |
| Why did an HTTP request fail?           | Daemon logs and the Wingman request ID                                |
| Why did a provider reject a model call? | Inspector diagnostics or the model-call API                           |
| Did Wingman retry the call?             | Model-call records with the same run and step, but different attempts |
| Why did a stream stop?                  | The failure body, transport operation, and cause chain                |
| Why was a failed attempt not retried?   | The attempt's `retry.decision` and `retry.reason`                     |
| Where did the model spend time?         | Per-attempt timing milestones and duration                            |
| How much did a call use?                | Token usage and estimated cost in the inspector                       |

A model call is one physical attempt to contact a provider.
A run can contain several steps, and a step can contain several attempts.
Each attempt has its own model-call ID and diagnostic record.

## Inspect a Failed Model Call

1. Open the affected Session in the Console.
2. Open **Inspector** from the context-usage indicator.
3. Find the failed attempt under **Model calls**.
4. Select **Copy diagnostics**.
5. Paste the result into a local text editor.
6. Read the error summary, then the `failure` object in the JSON snapshot.

The snapshot contains correlation IDs, timing, usage, build information, request structure, and captured failure evidence.
Request structure includes message counts and tool-schema hashes. It does not include the complete outbound request.

Provider evidence can contain quoted input, generated output, account details, or URLs.
Wingman redacts known credentials, but this operation does not remove all private content.
Before you share a snapshot, review its contents.

### Example: Exhausted API Credits

A provider can accept the HTTP connection, then send an error event.
In this case, `http_status: 200` does not mean that the model call succeeded.

This abbreviated example identifies a quota failure:

```json
{
  "stage": "stream",
  "http_status": 200,
  "category": "quota",
  "retryable": false,
  "event": "error",
  "code": "credit_balance_exhausted",
  "type": "insufficient_quota",
  "message": "You have no credits remaining. Add credits to continue using the API.",
  "output_started": false,
  "transport": {
    "kind": "http",
    "operation": "read",
    "phase": "receive",
    "delivery": "accepted",
    "recovery": "fail"
  }
}
```

For this failure, add credits to the provider account that owns the API key.
A different model on the same account can have the same failure.

## Read Daemon Logs

Wingman writes logs to standard error. The default format is JSON, and the default level is `info`.
Model-call diagnostics do not require the `debug` level.

### Foreground Server

To use readable text logs, start the server with this command:

```bash
wingman serve --log-format text --log-level info
```

To capture debug logs in a file, use this command:

```bash
wingman serve --log-format json --log-level debug 2>wingman-debug.jsonl
```

The server writes one JSON object per log line.
Accepted levels are `debug`, `info`, `warn`, and `error`.

### Managed Service

To read recent logs from the managed daemon, run:

```bash
wingman api get /logs
```

To print the original log lines, use `jq`:

```bash
wingman api get /logs | jq -r '.[].raw'
```

The endpoint returns the latest 500 log entries from the current process.
It does not provide a durable log archive. A daemon restart clears this buffer.
Managed daemons also write a private log file at `${XDG_STATE_HOME:-$HOME/.local/state}/wingman/wingman.log`.
The daemon keeps approximately 25 MiB of recent complete lines after the file reaches 50 MiB.
This file survives a daemon restart. A foreground `wingman serve` process does not write this file.
JSON entries include `time`, `level`, `msg`, and `attrs` when these fields are available.
Text entries retain the original line in `raw`.

On Linux, use the service journal for earlier process output:

```bash
journalctl --user -u wingman.service --since "30 minutes ago"
```

To change the managed-service log configuration, run:

```bash
wingman service start --log-format json --log-level debug
```

This command applies the runtime flags to the service definition.
Include any other runtime flags that the service needs, such as `--port` or `--db`.
For service commands and runtime flags, read the [CLI reference](/reference/cli).

### Interpret Log Entries

HTTP log entries include the method, path, route, status, response bytes, and duration in milliseconds.
They also include a Wingman `request_id`, the remote address, and the user agent.
Requests with query parameters include the query string.

Model-call completion entries include `model_call_id`, `step`, `attempt`, `status`, `provider_request_id`, and `duration_ms`.
Provider failures also include the error category, HTTP status, and retry eligibility.
Detailed provider bodies remain in the model-call record instead of the normal completion log.

Logs can contain private paths, query parameters, and error details.
The credential redaction for model-call diagnostics does not apply to every daemon log entry.

## Correlate Requests and Attempts

| Identifier             | Purpose                                                        |
| ---------------------- | -------------------------------------------------------------- |
| `session_id`           | Identifies the Session                                         |
| `run_id`               | Groups the work for one run                                    |
| `model_call_id`        | Identifies one physical provider attempt                       |
| `step` and `attempt`   | Distinguish steps and retries within the run                   |
| `assistant_message_id` | Links the attempt to its assistant message, when available     |
| `provider_request_id`  | Links the attempt to provider-side records or support requests |
| HTTP `X-Request-ID`    | Links a request to Wingman's API with its daemon log entry     |

The Wingman HTTP request ID and the provider request ID identify different requests.
The `failure.request_id` field contains the provider request ID.

To filter recent JSON logs for one model call, replace `mcl_example`:

```bash
wingman api get /logs |
  jq '.[] | select(.attrs.model_call_id == "mcl_example")'
```

## Inspect Diagnostics Through the API

To list the model calls for a Session, replace `ses_example`:

```bash
wingman api get /sessions/ses_example/model-calls
```

To select failed attempts and their evidence, use:

```bash
wingman api get /sessions/ses_example/model-calls |
  jq '.[] | select(.status == "failed") |
    {id, run_id, step, attempt, error_message, failure: .trace.failure}'
```

The API command connects to the managed daemon and supplies its authentication.
It does not select a separate foreground server.

For an explicit server, configure its URL and authentication as described in [Authentication](/concepts/authentication#direct-http-requests).
Then use:

```bash
curl -sS -u "$WINGMAN_AUTH" \
  "$WINGMAN_URL/sessions/ses_example/model-calls"
```

The API path starts at `/sessions`, not `/console/sessions`.
The model-call list returns the same captured evidence that the inspector copies.

## Diagnostic Fields

### Failure Evidence

| Field                        | Meaning                                                                                                |
| ---------------------------- | ------------------------------------------------------------------------------------------------------ |
| `stage`                      | `prepare`, `request`, `response`, or `stream`                                                          |
| `route`, `protocol`, `model` | The selected route, wire protocol, and effective model                                                 |
| `endpoint`                   | The provider URL without user information, query parameters, or fragment                               |
| `settings`                   | Selected effective generation parameters, including token limits and supported reasoning values        |
| `http_status`                | The provider HTTP status, when a response exists                                                       |
| `category`                   | Wingman's provider-neutral failure category                                                            |
| `classification`             | Additional detail: `context-overflow`, `payload-too-large`, or `incomplete-stream`                     |
| `event`                      | The triggering event name, when available                                                              |
| `code`, `type`, `param`      | Native provider identifiers and the affected parameter                                                 |
| `message`                    | The native provider explanation, when available                                                        |
| `body`                       | Original failure evidence, subject to credential redaction and size limits                             |
| `body_kind`                  | `response` for an HTTP rejection, `event` for a decoded frame, or `stream` for captured response bytes |
| `response_headers`           | Response headers, with credential-bearing values redacted                                              |
| `output_started`             | Whether Wingman emitted text, reasoning, or tool-input activity before the failure                     |
| `causes`                     | Wrapped or joined native errors, including their types, messages, and available codes                  |

The `body` field preserves unknown provider fields and non-JSON responses.
For a normal stream error, it contains the triggering frame's data.
For an interrupted or incomplete stream, it can contain a prefix of the response, including earlier output.
Truncation can leave an incomplete JSON document in this field.

Cause records can contain nested `causes` and a `stack` supplied by the original error formatter.
Wingman does not invent an original stack for Go errors that do not contain one.

### Retry and Transport Details

| Field                  | Meaning                                                               |
| ---------------------- | --------------------------------------------------------------------- |
| `retryable`            | Whether the failure category permits a retry                          |
| `retry_after_ms`       | The provider's suggested retry delay in milliseconds, when nonzero    |
| `rate_limit.limit`     | Provider limits, keyed by resource such as `tokens` or `requests`     |
| `rate_limit.remaining` | Remaining capacity for each reported resource                         |
| `rate_limit.reset`     | Provider reset values, retained in their original format              |
| `transport.kind`       | `http` for the current provider transport                             |
| `transport.operation`  | `request` or `read`                                                   |
| `transport.code`       | A native code such as `ECONNRESET`, `DNS_NOT_FOUND`, or `TIMEOUT`     |
| `transport.phase`      | The known failure phase: `prepare`, `connect`, `receive`, or `decode` |
| `transport.delivery`   | `not-sent`, `ambiguous`, `rejected`, or `accepted`                    |
| `transport.recovery`   | `retry-full` for an eligible dispatch retry, otherwise `fail`         |

`retryable: true` does not guarantee another attempt.
Attempt limits, cancellation, and the state of the stream also control retries.
Wingman does not automatically replay an established stream, even before visible output starts.

`retry.decision` records `scheduled` or `not_retried` for a failed attempt.
`scheduled` means Wingman recorded a retry plan. A later attempt shows that Wingman sent another request.
If cancellation stops the retry wait, Wingman changes the decision to `not_retried`.
`retry.delay_ms` records the selected wait in milliseconds, including a provider's `Retry-After` value.
`retry.reason` is `eligible`, `ineligible`, `attempt_limit`, `canceled`, or `established_stream`.
This decision describes the attempt. The `transport.recovery` field describes what the failure type permits.

`accepted` means that the provider established the HTTP stream. It does not prove that the model completed the request.
`ambiguous` means that Wingman cannot determine whether the provider received the request.
An absent `phase` means that the native error does not identify a more specific phase.

### Attempt Timing

`timing` contains elapsed milliseconds from the start of one physical attempt:

| Field               | Meaning                                                         |
| ------------------- | --------------------------------------------------------------- |
| `first_response_ms` | Wingman received the provider's stream start.                   |
| `first_activity_ms` | Wingman first received text, reasoning, or tool-input activity. |
| `first_answer_ms`   | Wingman first received answer text.                             |

The model-call start and completion timestamps give the full attempt duration.
The inspector calculates answer tokens per second from visible output tokens and the time after the first answer.
It subtracts reported reasoning tokens from output tokens.
If the provider reported no visible output tokens or there was no answer, the inspector omits that rate.
An absent milestone means that the activity did not occur or its time was not captured.
For example, a failed attempt before output has no `first_activity_ms`.
Each retry starts a new clock, so backoff time does not inflate an attempt's latency.

### Failure Categories

| Category or classification        | Interpretation                                                                             |
| --------------------------------- | ------------------------------------------------------------------------------------------ |
| `quota`                           | Exhausted credits or an account limit. Not retryable.                                      |
| `rate_limit`                      | Temporary throttling. Retry-eligible before an established stream.                         |
| `authentication`, `authorization` | Invalid credentials or denied access. Not retryable.                                       |
| `content_policy`                  | A provider policy rejection. Not retryable.                                                |
| `invalid_request`                 | A deterministic request rejection. Not retryable.                                          |
| `context-overflow`                | The input exceeds the provider's context limit.                                            |
| `payload-too-large`               | The request payload exceeds a size limit.                                                  |
| `unavailable`, `timeout`          | A transient provider failure or timeout. Retry-eligible before an established stream.      |
| `transport`                       | A connection or response-read failure. Retry eligibility also depends on the stream state. |
| `provider`                        | An unrecognized provider failure. Retry-eligible before an established stream.             |
| `decoding`                        | Invalid provider output or a missing completion event. Not automatically retried.          |
| `cancellation`                    | Cancellation of the call. Not retryable.                                                   |

HTTP status alone does not determine the category.
For example, HTTP 429 can mean temporary throttling or exhausted account credits.
Wingman uses native codes and messages to distinguish these failures.
Explicit quota evidence takes precedence over throttling evidence.

## Retention, Redaction, and Limits

Persistent Sessions retain diagnostics with each model-call record.
These records survive daemon restarts. Historical calls contain only the evidence that their original server captured.
Restarting a newer server cannot restore details that an older server omitted.

Ephemeral runs do not provide persistent Session records for the inspector.
Recent daemon logs have separate retention: the process keeps only its latest 500 entries.

Wingman removes credentials from known request headers, URL credentials, and query values when these values appear in captured evidence.
It also redacts credential-bearing response headers and recognized credential patterns.
This redaction preserves ordinary provider messages and schema identifiers.
The copied evidence can still contain private conversation content.

| Capture item                     | Limit                                 |
| -------------------------------- | ------------------------------------- |
| Failure body or stream prefix    | 64 KiB                                |
| Individual diagnostic text field | 2 KiB                                 |
| Response headers                 | 64 names and a 16 KiB text budget     |
| Individual header name           | 256 bytes                             |
| Individual header value          | 2 KiB                                 |
| Native causes                    | 16 records, with a maximum depth of 8 |
| Cause type                       | 256 bytes                             |
| Cause message                    | 2 KiB                                 |
| Available stack text per cause   | 8 KiB                                 |

`redacted` indicates that Wingman replaced captured values.
`truncated` indicates that at least one capture limit removed data.
`detail_omitted` indicates unavailable detail, such as an incomplete read of an HTTP error response.
Older records can also use this flag for omitted provider text.
An absent field means that the call did not provide that information or the server did not capture it.

## Diagnose Server Availability

To inspect the managed service, run:

```bash
wingman service status
```

To inspect readiness through the authenticated API, run:

```bash
wingman api get /ready
```

The public `/health` endpoint reports process health.
The authenticated `/ready` endpoint reports whether the daemon is ready to serve requests.
Provider credentials and account credits can still fail after the server becomes ready.

If the inspector lacks expected details after an upgrade, compare the snapshot's `build` object with `wingman version`.
The snapshot identifies the executable that captured the call.
A newer Console cannot add evidence to a call that an older daemon recorded.
