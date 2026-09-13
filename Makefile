.PHONY: run test dev-up dev-down dev-restart dev-status dev-logs

# Keep build artifacts out of the repository and support restricted workspaces.
GOCACHE ?= /tmp/moderatosaurusrex-go-build
GOMODCACHE ?= /tmp/moderatosaurusrex-go-mod
export GOCACHE GOMODCACHE

# Run the bot in the foreground. Ctrl+C stops it cleanly.
run:
	set -a; . ./.env; set +a; go run ./cmd/moderatosaurusrex

test:
	go test ./...

# Manage a local background bot process. Its output is kept in .dev/bot.log.
dev-up:
	./scripts/dev.sh up

dev-down:
	./scripts/dev.sh down

dev-restart:
	./scripts/dev.sh restart

dev-status:
	./scripts/dev.sh status

dev-logs:
	./scripts/dev.sh logs
