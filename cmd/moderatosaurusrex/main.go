package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct{ token, applicationID, guildID, databaseURL string }
type app struct {
	pool    *pgxpool.Pool
	session *discordgo.Session
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		fatal(fmt.Errorf("connect to PostgreSQL: %w", err))
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		fatal(fmt.Errorf("migrate PostgreSQL: %w", err))
	}
	session, err := discordgo.New("Bot " + cfg.token)
	if err != nil {
		fatal(fmt.Errorf("create Discord session: %w", err))
	}
	defer session.Close()
	a := &app{pool: pool, session: session}
	session.AddHandler(a.handleInteraction)
	if err := session.Open(); err != nil {
		fatal(fmt.Errorf("open Discord gateway: %w", err))
	}
	if err := registerCommands(session, cfg.applicationID, cfg.guildID); err != nil {
		fatal(fmt.Errorf("register slash commands: %w", err))
	}
	go a.runScheduler(ctx)
	slog.Info("Moderatosaurus Rex is running", "command_scope", scope(cfg.guildID))
	<-ctx.Done()
	slog.Info("shutting down")
}

func loadConfig() (config, error) {
	cfg := config{strings.TrimSpace(os.Getenv("DISCORD_TOKEN")), strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")), strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")), strings.TrimSpace(os.Getenv("DATABASE_URL"))}
	if cfg.token == "" {
		return config{}, errors.New("DISCORD_TOKEN is required")
	}
	if cfg.applicationID == "" {
		return config{}, errors.New("DISCORD_APPLICATION_ID is required")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func (a *app) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		a.processDueEvents(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (a *app) processDueEvents(ctx context.Context) {
	due, err := a.dueReminders(ctx)
	if err != nil {
		slog.Error("find due reminders", "error", err)
		return
	}
	for _, event := range due {
		if event.Visibility == "public" {
			if event.VoiceChannelID == "" {
				slog.Error("public session has no voice channel", "session_id", event.ID)
				continue
			}
			if _, err := a.session.ChannelMessageSend(event.VoiceChannelID, "**"+event.Title+"** starts in 15 minutes."); err != nil {
				slog.Error("send public session reminder", "session_id", event.ID, "error", err)
			}
			continue
		}
		users, err := a.participantIDs(ctx, event.ID)
		if err != nil {
			slog.Error("load reminder participants", "error", err)
			continue
		}
		for _, id := range users {
			dm, err := a.session.UserChannelCreate(id)
			if err != nil {
				slog.Error("open reminder DM", "user_id", id, "error", err)
				continue
			}
			if _, err := a.session.ChannelMessageSend(dm.ID, "Your play session **"+event.Title+"** starts in 15 minutes."); err != nil {
				slog.Error("send reminder DM", "user_id", id, "error", err)
			}
		}
	}
	archived, err := a.archiveExpired(ctx)
	if err != nil {
		slog.Error("archive expired events", "error", err)
		return
	}
	for _, event := range archived {
		if event.VoiceChannelID != "" {
			if _, err := a.session.ChannelDelete(event.VoiceChannelID); err != nil {
				slog.Error("delete expired session voice channel", "session_id", event.ID, "error", err)
				continue
			}
		}
		if err := a.archiveEvent(ctx, event.ID); err != nil {
			slog.Error("archive expired session", "session_id", event.ID, "error", err)
		}
	}
}
func scope(guildID string) string {
	if guildID == "" {
		return "global"
	}
	return "guild"
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
