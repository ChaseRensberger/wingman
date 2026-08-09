# @wingman-actor/client

Typed REST and server-sent event client for a matching Wingman daemon release.

```sh
npm install @wingman-actor/client
```

```ts
import { apiData, createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({ baseUrl: "http://localhost:2323" });
const sessions = await apiData(client.GET("/sessions"));
```

Use `streamRun` for ephemeral runs and `streamSessionEvents` for persistent
session events. See the [TypeScript SDK guide](https://docs.wingman.actor/build-clients/typescript-sdk/)
for authentication, streaming recovery, and compatibility details.
