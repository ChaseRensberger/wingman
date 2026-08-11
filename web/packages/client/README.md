# @wingman-actor/client

Typed REST and server-sent event client for a matching Wingman daemon release.

```sh
npm install @wingman-actor/client
```

```ts
import { createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: "http://localhost:2323",
  password: process.env.WINGMAN_DAEMON_PASSWORD,
  clientName: "my_app",
});

const sessions = await client.sessions.list();
```

Resource methods return data and throw `APIError` for HTTP errors. Use
`client.run.stream` for ephemeral runs and `client.sessions.streamEvents` for
persistent session events. See the [TypeScript SDK guide](https://docs.wingman.actor/build-clients/typescript-sdk/)
for streaming recovery and compatibility details.

Use `newMessageAdmission` and `client.sessions.admit` for retry-safe persistent
message admission.
