package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func registerCommands(s *discordgo.Session, appID, guildID string) error {
	commands := []*discordgo.ApplicationCommand{
		{Name: "ping", Description: "Check whether Moderatosaurus Rex is online."},
		{Name: "lfp", Description: "List public packs looking for players."},
		{Name: "event", Description: "Create and manage pack events.", Options: []*discordgo.ApplicationCommandOption{
			{Name: "create", Description: "Create a pack event.", Type: discordgo.ApplicationCommandOptionSubCommand, Options: []*discordgo.ApplicationCommandOption{
				{Name: "title", Description: "Name for this pack.", Type: 3, Required: true}, {Name: "when", Description: "Your local ISO-8601 time, e.g. 2026-09-13T20:00+02:00.", Type: 3, Required: true}, {Name: "visibility", Description: "Who can discover this event.", Type: 3, Required: true, Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "Public", Value: "public"}, {Name: "Private", Value: "private"}}}, {Name: "server", Description: "Desired Evrima server.", Type: 3, Required: true}, {Name: "species", Description: "Optional desired dinosaur species.", Type: 3}}},
			{Name: "join", Description: "Join a public event.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID from /lfp.", Type: 3, Required: true}}},
			{Name: "join-private", Description: "Join a private event by invite code.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "invite_code", Description: "Invite code from the creator.", Type: 3, Required: true}}},
			{Name: "leave", Description: "Leave an event.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID.", Type: 3, Required: true}}},
			{Name: "close", Description: "Close your event and remove it from /lfp.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID.", Type: 3, Required: true}}},
			{Name: "configure-channel", Description: "Set the channel for event announcements and reminders.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "channel", Description: "Channel for events and reminders.", Type: 7, Required: true}}},
		}},
	}
	for _, c := range commands {
		if _, err := s.ApplicationCommandCreate(appID, guildID, c); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		a.handleCommand(i)
	case discordgo.InteractionMessageComponent:
		a.handleButton(i)
	}
}
func userID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}
func (a *app) handleCommand(i *discordgo.InteractionCreate) {
	d := i.ApplicationCommandData()
	if d.Name == "ping" {
		a.reply(i, "Roar! Moderatosaurus Rex is online.", true)
		return
	}
	if i.GuildID == "" {
		a.reply(i, "This command can only be used in a server.", true)
		return
	}
	if d.Name == "lfp" {
		a.list(i)
		return
	}
	if d.Name != "event" || len(d.Options) == 0 {
		return
	}
	sub := d.Options[0]
	opts := map[string]string{}
	for _, o := range sub.Options {
		opts[o.Name] = fmt.Sprint(o.Value)
	}
	switch sub.Name {
	case "configure-channel":
		a.configure(i, opts["channel"])
	case "create":
		a.create(i, opts)
	case "join":
		a.join(i, opts["event_id"])
	case "join-private":
		a.joinPrivate(i, strings.ToUpper(opts["invite_code"]))
	case "leave":
		a.leave(i, opts["event_id"])
	case "close":
		a.close(i, opts["event_id"])
	}
}
func (a *app) configure(i *discordgo.InteractionCreate, channel string) {
	if i.Member == nil || i.Member.Permissions&discordgo.PermissionManageServer == 0 {
		a.reply(i, "You need the Manage Server permission to configure the event channel.", true)
		return
	}
	if err := a.setEventChannel(context.Background(), i.GuildID, channel); err != nil {
		a.fail(i, err)
		return
	}
	a.reply(i, "Event announcements and reminders will be sent to <#"+channel+">.", true)
}
func (a *app) create(i *discordgo.InteractionCreate, o map[string]string) {
	if _, err := a.eventChannel(context.Background(), i.GuildID); err != nil {
		a.reply(i, "An administrator must first run `/event configure-channel`.", true)
		return
	}
	when, err := parseEventTime(o["when"])
	if err != nil {
		a.reply(i, "Use an ISO-8601 time including your UTC offset, for example `2026-09-13T20:00+02:00`.", true)
		return
	}
	if when.Before(time.Now()) {
		a.reply(i, "The event time must be in the future.", true)
		return
	}
	e, err := a.createEvent(context.Background(), event{GuildID: i.GuildID, CreatorID: userID(i), Title: o["title"], Visibility: o["visibility"], GameServer: o["server"], Species: o["species"], StartsAt: when.UTC()})
	if err != nil {
		a.fail(i, err)
		return
	}
	if e.Visibility == "private" {
		dm, err := a.session.UserChannelCreate(userID(i))
		if err == nil {
			_, err = a.session.ChannelMessageSend(dm.ID, "Your private **"+e.Title+"** event is ready. Share invite code `"+e.InviteCode+"`. Participants join with `/event join-private invite_code:"+e.InviteCode+"`.")
		}
		if err != nil {
			a.reply(i, "I could not DM you the private invite code. Enable DMs from server members and try again.", true)
			return
		}
		a.reply(i, "I sent your private event invite code by DM.", true)
		return
	}
	channel, _ := a.eventChannel(context.Background(), i.GuildID)
	_, err = a.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{Content: eventText(e), Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.Button{Label: "Join pack", Style: discordgo.PrimaryButton, CustomID: "join:" + e.ID}, discordgo.Button{Label: "Leave pack", Style: discordgo.SecondaryButton, CustomID: "leave:" + e.ID}}}}})
	if err != nil {
		a.fail(i, err)
		return
	}
	a.reply(i, "Your public event is live in the configured event channel.", true)
}

func parseEventTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00"} {
		when, err := time.Parse(layout, value)
		if err == nil {
			return when, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid event time")
}
func eventText(e event) string {
	species := "Any species"
	if e.Species != "" {
		species = e.Species
	}
	return fmt.Sprintf("**%s**\nStarts <t:%d:F> (<t:%d:R>)\nServer: %s · Species: %s\nEvent ID: `%s`", e.Title, e.StartsAt.Unix(), e.StartsAt.Unix(), e.GameServer, species, e.ID)
}
func (a *app) list(i *discordgo.InteractionCreate) {
	events, err := a.publicEvents(context.Background(), i.GuildID)
	if err != nil {
		a.fail(i, err)
		return
	}
	if len(events) == 0 {
		a.reply(i, "No public packs are looking for players right now.", true)
		return
	}
	lines := make([]string, 0, len(events))
	for _, e := range events {
		lines = append(lines, eventText(e))
	}
	a.reply(i, "**Looking for pack**\n\n"+strings.Join(lines, "\n\n"), true)
}
func (a *app) joinPrivate(i *discordgo.InteractionCreate, code string) {
	e, err := a.eventByCode(context.Background(), i.GuildID, code)
	if err != nil {
		a.reply(i, "That private invite code is invalid or the event has closed.", true)
		return
	}
	a.joinEvent(i, e.ID)
}
func (a *app) join(i *discordgo.InteractionCreate, id string) {
	e, err := a.eventByID(context.Background(), i.GuildID, id)
	if err != nil {
		a.reply(i, "That event no longer exists.", true)
		return
	}
	if e.Visibility != "public" {
		a.reply(i, "Use `/event join-private` with the invite code for private events.", true)
		return
	}
	a.joinEvent(i, e.ID)
}
func (a *app) joinEvent(i *discordgo.InteractionCreate, id string) {
	e, err := a.eventByID(context.Background(), i.GuildID, id)
	if err != nil {
		a.reply(i, "That event no longer exists.", true)
		return
	}
	joined, err := a.enrollEvent(context.Background(), id, userID(i))
	if err != nil {
		a.fail(i, err)
		return
	}
	if !joined {
		a.reply(i, "You are already in this pack.", true)
		return
	}
	channel, err := a.eventChannel(context.Background(), i.GuildID)
	if err == nil {
		_, err = a.session.ChannelMessageSend(channel, "<@"+e.CreatorID+"> <@"+userID(i)+"> joined **"+e.Title+"**.")
	}
	if err != nil {
		slog.Error("announce participant", "error", err)
	}
	a.reply(i, "You joined **"+e.Title+"**.", true)
}
func (a *app) leave(i *discordgo.InteractionCreate, id string) {
	left, err := a.leaveEvent(context.Background(), id, userID(i))
	if err != nil {
		a.fail(i, err)
		return
	}
	if !left {
		a.reply(i, "You are not a participant, or you are the creator. Creators can use `/event close`.", true)
		return
	}
	a.reply(i, "You left the event.", true)
}
func (a *app) close(i *discordgo.InteractionCreate, id string) {
	closed, err := a.closeEvent(context.Background(), id, userID(i))
	if err != nil {
		a.fail(i, err)
		return
	}
	if !closed {
		a.reply(i, "Only the event creator can close an active event.", true)
		return
	}
	a.reply(i, "Your event is closed and no longer listed in /lfp.", true)
}
func (a *app) handleButton(i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) != 2 {
		return
	}
	switch parts[0] {
	case "join":
		a.join(i, parts[1])
	case "leave":
		a.leave(i, parts[1])
	}
}
func (a *app) reply(i *discordgo.InteractionCreate, text string, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: text, Flags: flags}}); err != nil {
		slog.Error("respond to interaction", "error", err)
	}
}
func (a *app) fail(i *discordgo.InteractionCreate, err error) {
	slog.Error("interaction failed", "error", err)
	a.reply(i, "Something went wrong. Please try again.", true)
}
