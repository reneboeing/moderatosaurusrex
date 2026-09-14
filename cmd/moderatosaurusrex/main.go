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
	_ "time/tzdata"

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
				categoryID, err := a.sessionCategory(ctx, event.GuildID)
				if err != nil || categoryID == "" {
					slog.Error("public session has no configured category", "session_id", event.ID, "error", err)
					_ = a.retryReminder(ctx, event.ID)
					continue
				}
				voiceChannel, err := a.createSessionVoiceChannel(event.GuildID, categoryID, event.Title)
				if err != nil {
					slog.Error("create public session voice channel", "session_id", event.ID, "error", err)
					_ = a.retryReminder(ctx, event.ID)
					continue
				}
				event.VoiceChannelID = voiceChannel.ID
				if err := a.setVoiceChannel(ctx, event.ID, voiceChannel.ID); err != nil {
					_, _ = a.session.ChannelDelete(voiceChannel.ID)
					slog.Error("store public session voice channel", "session_id", event.ID, "error", err)
					_ = a.retryReminder(ctx, event.ID)
					continue
				}
				if err := a.announcePublicSession(event); err != nil {
					_ = a.setVoiceChannel(ctx, event.ID, "")
					_, _ = a.session.ChannelDelete(voiceChannel.ID)
					slog.Error("announce public session", "session_id", event.ID, "error", err)
					_ = a.retryReminder(ctx, event.ID)
					continue
				}
			}
			if event.VoiceChannelID == "" {
				slog.Error("public session has no voice channel", "session_id", event.ID)
				continue
			}
			if _, err := a.session.ChannelMessageSend(event.VoiceChannelID, "**"+event.Title+"** starts in 15 minutes."); err != nil {
				slog.Error("send public session reminder", "session_id", event.ID, "error", err)
			}
			continue
		}
		if event.VoiceChannelID == "" {
			categoryID, err := a.sessionCategory(ctx, event.GuildID)
			if err != nil || categoryID == "" || event.RoleID == "" {
				slog.Error("private session has no configured category or role", "session_id", event.ID, "error", err)
				_ = a.retryReminder(ctx, event.ID)
				continue
			}
			voiceChannel, err := a.createPrivateSessionVoiceChannel(event, categoryID)
			if err != nil {
				slog.Error("create private session voice channel", "session_id", event.ID, "error", err)
				_ = a.retryReminder(ctx, event.ID)
				continue
			}
			event.VoiceChannelID = voiceChannel.ID
			if err := a.setVoiceChannel(ctx, event.ID, voiceChannel.ID); err != nil {
				_, _ = a.session.ChannelDelete(voiceChannel.ID)
				slog.Error("store private session voice channel", "session_id", event.ID, "error", err)
				_ = a.retryReminder(ctx, event.ID)
				continue
			}
		}
		users, err := a.participantIDs(ctx, event.ID)
		if err != nil {
			slog.Error("load private session participants", "session_id", event.ID, "error", err)
			continue
		}
		mentions := make([]string, 0, len(users))
		for _, id := range users {
			mentions = append(mentions, "<@"+id+">")
		}
		message := strings.Join(mentions, " ") + " — **" + event.Title + "** starts in 15 minutes. This private voice channel is ready."
		if _, err := a.session.ChannelMessageSend(event.VoiceChannelID, message); err != nil {
			slog.Error("send private session reminder", "session_id", event.ID, "error", err)
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
		if event.RoleID != "" {
			if err := a.session.GuildRoleDelete(event.GuildID, event.RoleID); err != nil {
				slog.Error("delete expired private session role", "session_id", event.ID, "error", err)
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
