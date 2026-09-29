---
title: "Run in Docker"
description: "Run Wingman with Docker Compose."
---

# Run in Docker

Download the [Compose file](https://github.com/chaserensberger/wingman/blob/main/compose.yaml) to a directory.
In that directory, run:

```bash
read -r -s -p 'Wingman password: ' WINGMAN_PASSWORD
echo
export WINGMAN_PASSWORD
docker compose up -d
```

Open `http://127.0.0.1:2424/console/`. Enter the username `wingman` and your password.
Compose keeps your data in a volume.

The Compose file binds Wingman to localhost. For remote access, use a secure tunnel.
