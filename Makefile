.PHONY: run test

# Keep build artifacts out of the repository and support restricted workspaces.
GOCACHE ?= /tmp/moderatosaurusrex-go-build
GOMODCACHE ?= /tmp/moderatosaurusrex-go-mod
export GOCACHE GOMODCACHE

# Run the bot in the foreground. Ctrl+C stops it cleanly.
run:
	set -a; . ./.env; set +a; go run ./cmd/moderatosaurusrex

test:
	go test ./...
