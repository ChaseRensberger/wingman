---
title: "Run in Docker"
description: "Run Wingman with Docker Compose."
---

# Run in Docker

Download the [Compose file](https://github.com/chaserensberger/wingman/blob/main/compose.yaml) to a deployment directory.
Keep `compose.yaml` and a private `.env` file in that directory.
Put your password in `.env`:

```dotenv
WINGMAN_PASSWORD='replace-with-your-password'
```

Do not commit or share `.env`. Restrict access to the file:

```bash
chmod 600 .env
```

Compose passes the password from `.env` to the container. Docker administrators can inspect it.
From the deployment directory, run:

```bash
docker compose up -d
```

Open `http://127.0.0.1:2424/console/`. Enter the username `wingman` and your password.
Compose keeps your data in a volume.

The Compose file binds Wingman to localhost. For remote access, use a secure tunnel.

## Configuration

Keep configuration in `config/wingman.json` beside the Compose file. See [Global Config](/configure/config) for supported options.
The container does not read your host configuration automatically.

Create `config/wingman.json` before you start Compose. Make sure that the container user, UID `10001`, can read it.
Add this read-only mount under the service's `volumes` entry, alongside the data volume:

```yaml
    volumes:
      - wingman-data:/data
      - ./config/wingman.json:/home/wingman/.config/wingman/wingman.json:ro
```

Edit the host JSON file, then restart Wingman:

```bash
docker compose restart wingman
```

After `.env` or Compose file changes, run:

```bash
docker compose up -d
```

## Project Files

Host project directories are not available in the container unless you mount them.
To give Wingman access to a project, add a mount under the service's `volumes` entry:

```yaml
      - /absolute/host/project:/workspace
```

Use `/workspace` as the session's working directory.
This mount lets the container modify the host project files. The container user must have the required file permissions.
Existing sessions need a working directory that exists inside the container.
