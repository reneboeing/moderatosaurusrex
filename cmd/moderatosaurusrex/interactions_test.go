package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestConfigurationCommandVisibility(t *testing.T) {
	commands := sessionCommands()
	configurationCount := 0
	for _, command := range commands {
		if command.Name == "sessions" && command.DefaultMemberPermissions != nil {
			t.Fatal("public session commands must remain available to regular members")
		}
		for _, option := range command.Options {
			if !strings.HasPrefix(option.Name, "configure-") {
				continue
			}
			configurationCount++
			if command.Name != "sessions-admin" || command.DefaultMemberPermissions == nil || *command.DefaultMemberPermissions != discordgo.PermissionManageServer {
				t.Fatalf("%s must belong to the restricted admin command", option.Name)
			}
			if command.DMPermission == nil || *command.DMPermission {
				t.Fatal("configuration must be disabled in direct messages")
			}
		}
	}
	if configurationCount != 3 {
		t.Fatalf("got %d configuration commands, want 3", configurationCount)
	}
}

func TestSessionHelpPermissions(t *testing.T) {
	for _, locale := range []discordgo.Locale{discordgo.EnglishUS, discordgo.German} {
		for _, test := range []struct {
			name   string
			member *discordgo.Member
			admin  bool
		}{
			{name: "no member"},
			{name: "regular member", member: &discordgo.Member{Permissions: discordgo.PermissionViewChannel}},
			{name: "server manager", member: &discordgo.Member{Permissions: discordgo.PermissionManageServer}, admin: true},
			{name: "administrator", member: &discordgo.Member{Permissions: discordgo.PermissionAdministrator}, admin: true},
		} {
			t.Run(string(locale)+"/"+test.name, func(t *testing.T) {
				i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{Member: test.member, Locale: locale}}
				if canManageSessions(i) != test.admin {
					t.Fatal("unexpected session-management permission")
				}
				help := sessionHelp(i)
				if strings.Contains(help, "configure-") != test.admin || strings.Contains(help, "/sessions-admin") != test.admin {
					t.Fatalf("unexpected configuration visibility in help: %s", help)
				}
				if !strings.Contains(help, "/sessions create") {
					t.Fatal("public help is missing")
				}
			})
		}
	}
}

type interactionTransport func(*http.Request) (*http.Response, error)

func (f interactionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConfigurationRejectsRegularMembers(t *testing.T) {
	for _, subcommand := range []string{"configure-category", "configure-timezone", "configure-private"} {
		t.Run(subcommand, func(t *testing.T) {
			replied := false
			session, err := discordgo.New("Bot test")
			if err != nil {
				t.Fatal(err)
			}
			session.Client = &http.Client{Transport: interactionTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(r.URL.Path, "/callback") {
					t.Fatalf("unexpected Discord request: %s", r.URL.Path)
				}
				var response discordgo.InteractionResponse
				if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response.Data == nil || !strings.Contains(response.Data.Content, "Manage Server") || response.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
					t.Fatalf("expected private permission denial, got %+v", response.Data)
				}
				replied = true
				return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})}
			a := &app{session: session}
			a.handleCommand(&discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
				Type: discordgo.InteractionApplicationCommand,
				ID:   "interaction", Token: "test", GuildID: "guild", Member: &discordgo.Member{},
				Data: discordgo.ApplicationCommandInteractionData{Name: "sessions-admin", Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: subcommand, Type: discordgo.ApplicationCommandOptionSubCommand}}},
			}})
			if !replied {
				t.Fatal("configuration command did not reject the regular member")
			}
		})
	}
}

func TestMemberCanManageRoles(t *testing.T) {
	roles := []*discordgo.Role{
		{ID: "guild", Permissions: discordgo.PermissionViewChannel},
		{ID: "manager", Permissions: discordgo.PermissionManageRoles},
		{ID: "admin", Permissions: discordgo.PermissionAdministrator},
	}
	for _, test := range []struct {
		name   string
		member *discordgo.Member
		want   bool
	}{
		{name: "missing member", want: false},
		{name: "no permission", member: &discordgo.Member{}, want: false},
		{name: "manage roles", member: &discordgo.Member{Roles: []string{"manager"}}, want: true},
		{name: "administrator", member: &discordgo.Member{Roles: []string{"admin"}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := memberCanManageRoles("guild", test.member, roles); got != test.want {
				t.Fatalf("memberCanManageRoles() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestVoiceChannelDeletionFailureMessageNamesChannel(t *testing.T) {
	message := voiceChannelDeletionFailureMessage("night-roam")
	if !strings.Contains(message, "**night-roam**") || !strings.Contains(message, "Manage Channels") {
		t.Fatalf("deletion-failure message = %q; want channel name and missing permission", message)
	}
}

func TestParseEventTime(t *testing.T) {
	for _, input := range []string{"2026-09-13T20:43+02:00", "2026-09-13T20:43:00+02:00"} {
		got, err := parseEventTime(input, time.FixedZone("UTC+2", 2*60*60))
		if err != nil {
			t.Fatalf("parseEventTime(%q): %v", input, err)
		}
		if got.UTC().Format(time.RFC3339) != "2026-09-13T18:43:00Z" {
			t.Fatalf("parseEventTime(%q) returned unexpected time %s", input, got)
		}
	}
}

func TestParseEventTimeRelative(t *testing.T) {
	location := time.FixedZone("UTC+2", 2*60*60)
	got, err := parseClock(time.Date(2026, 9, 13, 10, 0, 0, 0, location), "8pm", location)
	if err != nil || got.Hour() != 20 {
		t.Fatalf("parseClock(8pm) = %s, %v", got, err)
	}
}

func TestParseEventTimeGermanRelative(t *testing.T) {
	location := time.FixedZone("UTC+2", 2*60*60)
	got, err := parseEventTime("morgen 20 uhr", location)
	if err != nil || got.Hour() != 20 {
		t.Fatalf("parseEventTime(morgen 20 uhr) = %s, %v", got, err)
	}
}
