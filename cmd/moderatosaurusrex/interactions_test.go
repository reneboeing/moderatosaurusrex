package main

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

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
