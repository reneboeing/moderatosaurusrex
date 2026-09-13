# Moderatosaurus Rex

Moderatosaurus Rex is a Discord bot for Crunch's community. It will help players
of *The Isle: Evrima* find groups to play with.

This first harness connects to Discord and exposes a `/ping` slash command. It
answers with `Roar! Moderatosaurus Rex is online.`

## Configuration

For local development, fill in the git-ignored `.env` file. For other
environments, set these environment variables directly:

- `DISCORD_TOKEN` — bot token from the Discord Developer Portal.
- `DISCORD_APPLICATION_ID` — Discord application's Application ID.
- `DISCORD_GUILD_ID` — optional development-server ID. Commands register there
  immediately; omit it for global commands.
- `DATABASE_URL` — PostgreSQL connection string. The bot creates its required
  tables on startup with the configured database user.

Invite the bot with the `bot` and `applications.commands` OAuth2 scopes. The
bot needs no privileged gateway intents. For public play sessions, grant it
**Manage Channels** and permission to send messages in voice-channel chat.

## Local development

Keep PostgreSQL running in Docker, then run the bot from a terminal you keep
open. The bot stays attached to that terminal: its logs appear there, and
`Ctrl+C` cleanly stops it. Starting it again is the restart procedure.

```sh
make run
```

Run the test suite with `make test`.

For a managed background process, use the development commands instead. They
build the binary, keep the process ID in the git-ignored `.dev` directory, and
write output to `.dev/bot.log`:

```sh
make dev-up       # build and start
make dev-logs     # follow logs (Ctrl+C stops only the log view)
make dev-restart  # rebuild and restart after a code change
make dev-status
make dev-down
```

## Run with Docker

```sh
docker build -t moderatosaurusrex .
docker run --rm \
  -e DISCORD_TOKEN \
  -e DISCORD_APPLICATION_ID \
  -e DISCORD_GUILD_ID \
  -e DATABASE_URL \
  moderatosaurusrex
```

The final image is `scratch`-based and runs as the unprivileged numeric user
`65532:65532`.

## Published images

GitHub Actions tests every pull request. Pushes to `main` publish the `latest`
image to GitHub Container Registry, while a semantic version tag such as
`v0.1.0` also publishes versioned images:

```sh
docker pull ghcr.io/reneboeing/moderatosaurusrex:0.1.0
```

Published version tags also receive matching `major.minor`, `major`, and
`latest` tags. Each tag contains both `linux/amd64` and `linux/arm64` images,
so Docker and Portainer pull the correct architecture automatically. The package
can be made public from its package settings on GitHub if people should be able
to pull it without authenticating.

## Deploy with Portainer

Create a new **Stack** in Portainer, paste the following Compose definition,
then add the Discord values and `POSTGRES_PASSWORD` as environment variables in
the stack's environment-variable section. `DISCORD_GUILD_ID` is optional and
is useful while developing commands in a single server.

```yaml
services:
  moderatosaurusrex:
    image: ghcr.io/reneboeing/moderatosaurusrex:latest
    restart: unless-stopped
    environment:
      DISCORD_TOKEN: ${DISCORD_TOKEN}
      DISCORD_APPLICATION_ID: ${DISCORD_APPLICATION_ID}
      DISCORD_GUILD_ID: ${DISCORD_GUILD_ID}
      DATABASE_URL: postgres://moderatosaurusrex:${POSTGRES_PASSWORD}@postgres:5432/moderatosaurusrex?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: moderatosaurusrex
      POSTGRES_USER: moderatosaurusrex
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - moderatosaurusrex-postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U moderatosaurusrex -d moderatosaurusrex"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  moderatosaurusrex-postgres:
```

The bot makes an outbound connection to Discord, so this stack exposes no
network ports. If the GHCR package is private, configure GitHub Container
Registry credentials in Portainer before deploying; public packages need no
registry credentials.

## Play sessions

An administrator runs `/sessions configure-timezone` once per server. They can
also use `/sessions configure-channel` to choose a category or existing channel
alongside which public session voice channels should be created. Everyone then
uses `/sessions create`, chooses public or private, and completes a short form
with a session name, start time, optional Evrima server, and optional species.
Play-session times use the configured server timezone and accept inputs such as
`tomorrow 8pm`, `Friday 19:30`, and `2026-09-13 20:43`. Each public play session
gets a temporary voice channel. Its announcement, join notices, and reminder
are posted in that voice channel's chat, and it appears in `/sessions browse`.
Slash-command fallbacks are `/sessions join` and `/sessions leave`.

Private play sessions are not announced or listed. Their host receives an invite
code in a direct message, which players use with `/sessions join-private`. The
host is automatically a participant and can end the session with `/sessions
end`. Its 15-minute reminder is sent directly to every participant. Sessions
remain active after starting and are deleted when ended or eight hours after
their start time; associated public voice channels are deleted at the same time.
