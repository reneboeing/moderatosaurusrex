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

	"github.com/bwmarrin/discordgo"
)

type config struct {
	token         string
	applicationID string
	guildID       string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	session, err := discordgo.New("Bot " + cfg.token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create Discord session: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "ping" {
			return
		}

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Roar! Moderatosaurus Rex is online."},
		})
		if err != nil {
			slog.Error("respond to interaction", "error", err)
		}
	})

	if err := session.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "open Discord gateway: %v\n", err)
		os.Exit(1)
	}

	_, err = session.ApplicationCommandCreate(cfg.applicationID, cfg.guildID, &discordgo.ApplicationCommand{
		Name:        "ping",
		Description: "Check whether Moderatosaurus Rex is online.",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "register /ping command: %v\n", err)
		os.Exit(1)
	}
	if cfg.guildID == "" {
		slog.Info("Moderatosaurus Rex is running", "command_scope", "global")
	} else {
		slog.Info("Moderatosaurus Rex is running", "command_scope", "guild", "guild_id", cfg.guildID)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	slog.Info("shutting down")
}

func loadConfig() (config, error) {
	cfg := config{
		token:         strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		applicationID: strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")),
		guildID:       strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
	}

	if cfg.token == "" {
		return config{}, errors.New("DISCORD_TOKEN is required")
	}
	if cfg.applicationID == "" {
		return config{}, errors.New("DISCORD_APPLICATION_ID is required")
	}
	return cfg, nil
}
