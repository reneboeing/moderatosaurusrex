# Development workflow

Run PostgreSQL in Docker and run the bot directly on the host. Do not run the
bot itself in Docker for day-to-day debugging.

The git-ignored `.env` file holds the Discord and database configuration. Never
print its contents or commit it.

Manage the local bot only through these commands:

```sh
make dev-up       # build and start the background bot
make dev-restart  # rebuild and replace the managed bot
make dev-status   # report the managed bot PID
make dev-logs     # follow .dev/bot.log; Ctrl+C only stops log following
make dev-down     # stop the managed bot
```

The PID, binary, and log are stored in git-ignored `.dev/`. Before starting a
bot, run `make dev-status`; use `make dev-restart` for code changes. Do not use
`go run` with `nohup`, `setsid`, or ad-hoc background shell commands, and do
not leave duplicate bot processes running.

For a non-following log check, use `tail -n 100 .dev/bot.log` after confirming
the managed process is running. Run the test suite with `make test` before
committing Go changes.
