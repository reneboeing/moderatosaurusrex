# Moderatosaurus Rex

Moderatosaurus Rex is a Discord bot for Crunch's community. It will help players
of *The Isle: Evrima* find groups to play with.

This first harness connects to Discord and exposes a `/ping` slash command. It
answers with `Roar! Moderatosaurus Rex is online.`

## Configuration

Copy `.env.example` and set these environment variables:

- `DISCORD_TOKEN` — bot token from the Discord Developer Portal.
- `DISCORD_APPLICATION_ID` — Discord application's Application ID.
- `DISCORD_GUILD_ID` — optional development-server ID. When set, `/ping` is
  registered only in that server and becomes available immediately. Without it,
  the command is global and Discord can take time to propagate it.

Invite the bot with the `bot` and `applications.commands` OAuth2 scopes. The
bot needs no privileged gateway intents for this demo.

## Run locally

```sh
export DISCORD_TOKEN='your-bot-token'
export DISCORD_APPLICATION_ID='your-application-id'
export DISCORD_GUILD_ID='your-development-server-id' # optional
go run ./cmd/moderatosaurusrex
```

## Run with Docker

```sh
docker build -t moderatosaurusrex .
docker run --rm \
  -e DISCORD_TOKEN \
  -e DISCORD_APPLICATION_ID \
  -e DISCORD_GUILD_ID \
  moderatosaurusrex
```

The final image is `scratch`-based and runs as the unprivileged numeric user
`65532:65532`.

## Published images

GitHub Actions tests every pull request and builds the image for pushes to
`main`. Pushing a semantic version tag such as `v0.1.0` also publishes it to
GitHub Container Registry:

```sh
docker pull ghcr.io/reneboeing/moderatosaurusrex:0.1.0
```

Published version tags also receive matching `major.minor`, `major`, and
`latest` tags. The package can be made public from its package settings on
GitHub if people should be able to pull it without authenticating.

## Deploy with Portainer

Create a new **Stack** in Portainer, paste the following Compose definition,
then add the three Discord values as environment variables in the stack's
environment-variable section. `DISCORD_GUILD_ID` is optional and is useful
while developing commands in a single server.

```yaml
services:
  moderatosaurusrex:
    image: ghcr.io/reneboeing/moderatosaurusrex:latest
    restart: unless-stopped
    environment:
      DISCORD_TOKEN: ${DISCORD_TOKEN}
      DISCORD_APPLICATION_ID: ${DISCORD_APPLICATION_ID}
      DISCORD_GUILD_ID: ${DISCORD_GUILD_ID}
```

The bot makes an outbound connection to Discord, so this stack exposes no
network ports. If the GHCR package is private, configure GitHub Container
Registry credentials in Portainer before deploying; public packages need no
registry credentials.
