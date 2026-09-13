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
			{Name: "create", Description: "Open the guided event form.", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "join", Description: "Join a public event.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID from /lfp.", Type: 3, Required: true}}},
			{Name: "join-private", Description: "Join a private event by invite code.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "invite_code", Description: "Invite code from the creator.", Type: 3, Required: true}}},
			{Name: "leave", Description: "Leave an event.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID.", Type: 3, Required: true}}},
			{Name: "close", Description: "Close your event and remove it from /lfp.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "event_id", Description: "The event ID.", Type: 3, Required: true}}},
			{Name: "configure-channel", Description: "Set the channel for event announcements and reminders.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "channel", Description: "Channel for events and reminders.", Type: 7, Required: true}}},
			{Name: "configure-timezone", Description: "Set the server event timezone, e.g. Europe/Berlin.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "timezone", Description: "IANA timezone, e.g. Europe/Berlin.", Type: 3, Required: true}}},
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
	case discordgo.InteractionModalSubmit:
		a.handleModal(i)
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
	case "configure-timezone":
		a.configureTimezone(i, opts["timezone"])
	case "create":
		a.showCreateChoice(i)
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
func (a *app) configureTimezone(i *discordgo.InteractionCreate, timezone string) {
	if i.Member == nil || i.Member.Permissions&discordgo.PermissionManageServer == 0 {
		a.reply(i, "You need the Manage Server permission to configure the event timezone.", true)
		return
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		a.reply(i, "Use an IANA timezone such as `Europe/Berlin` or `America/New_York`.", true)
		return
	}
	if err := a.setTimezone(context.Background(), i.GuildID, timezone); err != nil {
		a.fail(i, err)
		return
	}
	a.reply(i, "Event times for this server now use `"+timezone+"`.", true)
}
func (a *app) showCreateChoice(i *discordgo.InteractionCreate) {
	a.replyWithComponents(i, "Choose who can discover this event:", []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Public event", Style: discordgo.PrimaryButton, CustomID: "create:public"},
		discordgo.Button{Label: "Private event", Style: discordgo.SecondaryButton, CustomID: "create:private"},
	}}}, true)
}
func (a *app) showCreateModal(i *discordgo.InteractionCreate, visibility string) {
	err := a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: "event-create:" + visibility, Title: "Create pack event", Components: []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "title", Label: "Pack name", Style: discordgo.TextInputShort, Required: true, MaxLength: 100}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "when", Label: "When (server timezone)", Style: discordgo.TextInputShort, Placeholder: "tomorrow 8pm, Friday 19:30, or 2026-09-13 20:43", Required: true, MaxLength: 64}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "server", Label: "Evrima server", Style: discordgo.TextInputShort, Required: true, MaxLength: 100}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "species", Label: "Desired species (optional)", Style: discordgo.TextInputShort, Required: false, MaxLength: 100}}},
	}}})
	if err != nil {
		slog.Error("open event form", "error", err)
	}
}
func modalValues(i *discordgo.InteractionCreate) map[string]string {
	values := map[string]string{}
	for _, row := range i.ModalSubmitData().Components {
		for _, component := range row.(*discordgo.ActionsRow).Components {
			input := component.(*discordgo.TextInput)
			values[input.CustomID] = strings.TrimSpace(input.Value)
		}
	}
	return values
}
func (a *app) handleModal(i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	parts := strings.Split(data.CustomID, ":")
	if len(parts) != 2 || parts[0] != "event-create" {
		return
	}
	a.create(i, modalValues(i), parts[1])
}
func (a *app) create(i *discordgo.InteractionCreate, o map[string]string, visibility string) {
	if channel, err := a.eventChannel(context.Background(), i.GuildID); err != nil || channel == "" {
		a.reply(i, "An administrator must first run `/event configure-channel`.", true)
		return
	}
	timezone, err := a.eventTimezone(context.Background(), i.GuildID)
	if err != nil {
		a.reply(i, "An administrator must first run `/event configure-timezone`.", true)
		return
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		a.fail(i, err)
		return
	}
	when, err := parseEventTime(o["when"], location)
	if err != nil {
		a.reply(i, "Try `tomorrow 8pm`, `Friday 19:30`, or `2026-09-13 20:43`.", true)
		return
	}
	if when.Before(time.Now()) {
		a.reply(i, "The event time must be in the future.", true)
		return
	}
	e, err := a.createEvent(context.Background(), event{GuildID: i.GuildID, CreatorID: userID(i), Title: o["title"], Visibility: visibility, GameServer: o["server"], Species: o["species"], StartsAt: when.UTC()})
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

func parseEventTime(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00"} {
		when, err := time.Parse(layout, value)
		if err == nil {
			return when, nil
		}
	}
	value = strings.ToLower(strings.TrimSpace(value))
	now := time.Now().In(location)
	for prefix, days := range map[string]int{"today ": 0, "tomorrow ": 1} {
		if clock, ok := strings.CutPrefix(value, prefix); ok {
			return parseClock(now.AddDate(0, 0, days), clock, location)
		}
	}
	for weekday, target := range map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday} {
		if clock, ok := strings.CutPrefix(value, weekday+" "); ok {
			days := (int(target) - int(now.Weekday()) + 7) % 7
			if days == 0 {
				days = 7
			}
			return parseClock(now.AddDate(0, 0, days), clock, location)
		}
	}
	for _, layout := range []string{"2006-01-02 15:04", "2 jan 15:04", "jan 2 15:04"} {
		if when, err := time.ParseInLocation(layout, value, location); err == nil {
			if !strings.Contains(layout, "2006") {
				when = time.Date(now.Year(), when.Month(), when.Day(), when.Hour(), when.Minute(), 0, 0, location)
				if when.Before(now) {
					when = when.AddDate(1, 0, 0)
				}
			}
			return when, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid event time")
}
func parseClock(day time.Time, value string, location *time.Location) (time.Time, error) {
	value = strings.ToUpper(strings.ReplaceAll(value, " ", ""))
	for _, layout := range []string{"15:04", "3PM", "3:04PM"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, location), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid clock")
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
	case "create":
		a.showCreateModal(i, parts[1])
	case "join":
		a.join(i, parts[1])
	case "leave":
		a.leave(i, parts[1])
	}
}
func (a *app) replyWithComponents(i *discordgo.InteractionCreate, text string, components []discordgo.MessageComponent, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: text, Components: components, Flags: flags}}); err != nil {
		slog.Error("respond to interaction", "error", err)
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
