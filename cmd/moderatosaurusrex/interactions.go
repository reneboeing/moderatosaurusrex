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
	previous, err := s.ApplicationCommands(appID, guildID)
	if err != nil {
		return err
	}
	for _, command := range previous {
		if command.Name == "ping" || command.Name == "lfp" || command.Name == "event" {
			if err := s.ApplicationCommandDelete(appID, guildID, command.ID); err != nil {
				return err
			}
		}
	}
	for _, c := range sessionCommands() {
		if _, err := s.ApplicationCommandCreate(appID, guildID, c); err != nil {
			return err
		}
	}
	return nil
}

func sessionCommands() []*discordgo.ApplicationCommand {
	permissions := int64(discordgo.PermissionManageServer)
	dmPermission := false
	return []*discordgo.ApplicationCommand{
		{Name: "sessions", Description: "Host, find, and manage Evrima play sessions.", Options: []*discordgo.ApplicationCommandOption{
			{Name: "help", Description: "Learn how to use play sessions.", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "browse", Description: "Browse public play sessions.", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "create", Description: "Open the guided play-session form.", Type: 1},
			{Name: "join", Description: "Choose a public play session to join.", Type: 1},
			{Name: "join-private", Description: "Enter a private-session invite code.", Type: 1},
			{Name: "leave", Description: "Choose a play session to leave.", Type: 1},
			{Name: "end", Description: "End a hosted session; admins may end any session.", Type: 1},
		}},
		{Name: "sessions-admin", Description: "Configure play sessions for this server.", DefaultMemberPermissions: &permissions, DMPermission: &dmPermission, Options: []*discordgo.ApplicationCommandOption{
			{Name: "configure-category", Description: "Set the category for public session voice channels.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "category", Description: "Category for public session voice channels.", Type: 7, ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildCategory}, Required: true}}},
			{Name: "configure-timezone", Description: "Set the server session timezone, e.g. Europe/Berlin.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "timezone", Description: "IANA timezone, e.g. Europe/Berlin.", Type: 3, Required: true}}},
			{Name: "configure-private", Description: "Enable or disable private play sessions.", Type: 1, Options: []*discordgo.ApplicationCommandOption{{Name: "enabled", Description: "Whether private play sessions are enabled.", Type: discordgo.ApplicationCommandOptionBoolean, Required: true}}},
		}},
	}
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
	if i.GuildID == "" {
		a.reply(i, "This command can only be used in a server.", true)
		return
	}
	if (d.Name != "sessions" && d.Name != "sessions-admin") || len(d.Options) == 0 {
		return
	}
	sub := d.Options[0]
	opts := map[string]string{}
	for _, o := range sub.Options {
		opts[o.Name] = fmt.Sprint(o.Value)
	}
	if d.Name == "sessions-admin" {
		switch sub.Name {
		case "configure-category":
			a.configureCategory(i, opts["category"])
		case "configure-timezone":
			a.configureTimezone(i, opts["timezone"])
		case "configure-private":
			a.configurePrivate(i, opts["enabled"] == "true")
		}
		return
	}
	switch sub.Name {
	case "help":
		a.showHelp(i)
	case "browse":
		a.list(i)
	case "create":
		a.showCreateChoice(i)
	case "join":
		a.showJoinEvents(i)
	case "join-private":
		a.showPrivateJoinModal(i)
	case "leave":
		a.showLeaveEvents(i)
	case "end":
		if opts["event_id"] != "" {
			a.close(i, opts["event_id"])
		} else {
			a.showCloseEvents(i)
		}
	}
}
func (a *app) showHelp(i *discordgo.InteractionCreate) {
	a.reply(i, sessionHelp(i), true)
}

func sessionHelp(i *discordgo.InteractionCreate) string {
	message := text(i,
		"**Moderatosaurus Rex — play sessions**\n"+
			"Use `/sessions browse` to find public roams and join from the list.\n"+
			"Use `/sessions create` to host a public or private session. Public sessions open a voice channel 15 minutes before they begin. Private hosts receive an invite code in the creation response; players join with `/sessions join-private`.\n"+
			"Use `/sessions leave` to leave and `/sessions end` to end a session you host. Server administrators can end any session.",
		"**Moderatosaurus Rex — Spielrunden**\n"+
			"Mit `/sessions browse` findest du öffentliche Runden und kannst direkt aus der Liste beitreten.\n"+
			"Mit `/sessions create` erstellst du eine öffentliche oder private Spielrunde. Für öffentliche Runden wird 15 Minuten vorher ein Sprachkanal geöffnet. Hosts privater Runden erhalten den Einladungscode in der Erstellungsantwort; Spieler treten mit `/sessions join-private` bei.\n"+
			"Mit `/sessions leave` verlässt du eine Runde; mit `/sessions end` beendest du eine von dir gehostete Runde. Server-Administratoren können jede Runde beenden.")
	if canManageSessions(i) {
		message += "\n" + text(i,
			"Admins: use `/sessions-admin configure-timezone` and `/sessions-admin configure-category` once per server; use `/sessions-admin configure-private` to manage private sessions.",
			"Admins: `/sessions-admin configure-timezone` und `/sessions-admin configure-category` werden pro Server einmal eingerichtet; `/sessions-admin configure-private` verwaltet private Spielrunden.")
	}
	return message
}
func (a *app) configureCategory(i *discordgo.InteractionCreate, category string) {
	if !canManageSessions(i) {
		a.reply(i, "You need the Manage Server permission to configure the session category.", true)
		return
	}
	selected, err := a.session.Channel(category)
	if err != nil || selected.Type != discordgo.ChannelTypeGuildCategory {
		a.reply(i, "Choose a server category for public session voice channels.", true)
		return
	}
	if err := a.setSessionCategory(context.Background(), i.GuildID, category); err != nil {
		a.fail(i, err)
		return
	}
	a.reply(i, text(i, "Public session voice channels will be created in <#"+category+">.", "Öffentliche Sprachkanäle für Spielrunden werden in <#"+category+"> erstellt."), true)
}
func (a *app) configureTimezone(i *discordgo.InteractionCreate, timezone string) {
	if !canManageSessions(i) {
		a.reply(i, text(i, "You need the Manage Server permission to configure the session timezone.", "Du benötigst die Berechtigung „Server verwalten“, um die Zeitzone für Spielrunden einzurichten."), true)
		return
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		a.reply(i, text(i, "Use an IANA timezone such as `Europe/Berlin` or `America/New_York`.", "Verwende eine IANA-Zeitzone wie `Europe/Berlin` oder `America/New_York`."), true)
		return
	}
	if err := a.setTimezone(context.Background(), i.GuildID, timezone); err != nil {
		a.fail(i, err)
		return
	}
	a.reply(i, text(i, "Play-session times for this server now use `"+timezone+"`.", "Zeiten für Spielrunden verwenden jetzt `"+timezone+"`."), true)
}
func (a *app) configurePrivate(i *discordgo.InteractionCreate, enabled bool) {
	if !canManageSessions(i) {
		a.reply(i, "You need the Manage Server permission to configure private sessions.", true)
		return
	}
	if enabled {
		canManageRoles, err := a.botCanManageRoles(i.GuildID)
		if err != nil || !canManageRoles {
			a.reply(i, "Private sessions cannot be enabled because I need the Manage Roles permission.", true)
			return
		}
	}
	if err := a.setPrivateSessionsEnabled(context.Background(), i.GuildID, enabled); err != nil {
		a.fail(i, err)
		return
	}
	if enabled {
		a.reply(i, "Private play sessions are now enabled.", true)
		return
	}
	a.reply(i, "Private play sessions are now disabled.", true)
}
func (a *app) showCreateChoice(i *discordgo.InteractionCreate) {
	enabled, err := a.privateSessionsEnabled(context.Background(), i.GuildID)
	if err != nil {
		a.fail(i, err)
		return
	}
	buttons := []discordgo.MessageComponent{discordgo.Button{Label: text(i, "Public session", "Öffentliche Spielrunde"), Style: discordgo.PrimaryButton, CustomID: "create:public"}}
	if enabled {
		buttons = append(buttons, discordgo.Button{Label: text(i, "Private session", "Private Spielrunde"), Style: discordgo.SecondaryButton, CustomID: "create:private"})
	}
	a.replyWithComponents(i, text(i, "Who can discover this play session?", "Wer kann diese Spielrunde finden?"), []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}, true)
}
func (a *app) showCreateModal(i *discordgo.InteractionCreate, visibility string) {
	if visibility == "private" {
		enabled, err := a.privateSessionsEnabled(context.Background(), i.GuildID)
		if err != nil {
			a.fail(i, err)
			return
		}
		if !enabled {
			a.reply(i, "Private play sessions are not enabled on this server.", true)
			return
		}
	}
	err := a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: "session-create:" + visibility, Title: text(i, "Host a play session", "Spielrunde erstellen"), Components: []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "title", Label: text(i, "Session name", "Name der Spielrunde"), Style: discordgo.TextInputShort, Required: true, MaxLength: 100}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "when", Label: text(i, "When (server timezone)", "Wann (Server-Zeitzone)"), Style: discordgo.TextInputShort, Placeholder: text(i, "tomorrow 8pm, Friday 19:30, or 2026-09-13 20:43", "morgen 20 Uhr, Freitag 19:30 oder 2026-09-13 20:43"), Required: true, MaxLength: 64}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "server", Label: text(i, "Evrima server (optional)", "Evrima-Server (optional)"), Style: discordgo.TextInputShort, Required: false, MaxLength: 100}}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "species", Label: text(i, "Desired species (optional)", "Gewünschte Spezies (optional)"), Style: discordgo.TextInputShort, Required: false, MaxLength: 100}}},
	}}})
	if err != nil {
		slog.Error("open session form", "error", err)
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
	if data.CustomID == "private-join" {
		a.joinPrivate(i, strings.ToUpper(modalValues(i)["invite_code"]))
		return
	}
	if len(parts) != 2 || parts[0] != "session-create" {
		return
	}
	a.create(i, modalValues(i), parts[1])
}
func (a *app) showPrivateJoinModal(i *discordgo.InteractionCreate) {
	enabled, err := a.privateSessionsEnabled(context.Background(), i.GuildID)
	if err != nil {
		a.fail(i, err)
		return
	}
	if !enabled {
		a.reply(i, "Private play sessions are not enabled on this server.", true)
		return
	}
	err = a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: "private-join", Title: text(i, "Join private session", "Privater Spielrunde beitreten"), Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "invite_code", Label: text(i, "Invite code", "Einladungscode"), Placeholder: "REX-1234abcd", Style: discordgo.TextInputShort, Required: true, MaxLength: 32}}}}}})
	if err != nil {
		slog.Error("open private join form", "error", err)
	}
}
func (a *app) create(i *discordgo.InteractionCreate, o map[string]string, visibility string) {
	if !a.deferReply(i, true) {
		return
	}
	categoryID := ""
	if visibility == "private" {
		enabled, err := a.privateSessionsEnabled(context.Background(), i.GuildID)
		if err != nil {
			a.failDeferred(i, err)
			return
		}
		if !enabled {
			a.editReply(i, "Private play sessions are not enabled on this server.")
			return
		}
	}
	if visibility == "public" || visibility == "private" {
		var err error
		categoryID, err = a.sessionCategory(context.Background(), i.GuildID)
		if err != nil || categoryID == "" {
			a.editReply(i, "Ask a server administrator to configure the session category first.")
			return
		}
	}
	timezone, err := a.eventTimezone(context.Background(), i.GuildID)
	if err != nil {
		a.editReply(i, "Ask a server administrator to configure the session timezone first.")
		return
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		a.failDeferred(i, err)
		return
	}
	when, err := parseEventTime(o["when"], location)
	if err != nil {
		a.editReply(i, "Try `tomorrow 8pm`, `Friday 19:30`, or `2026-09-13 20:43`.")
		return
	}
	if when.Before(time.Now()) {
		a.editReply(i, "The play-session time must be in the future.")
		return
	}
	e, err := a.createEvent(context.Background(), event{GuildID: i.GuildID, CreatorID: userID(i), Title: o["title"], Visibility: visibility, GameServer: o["server"], Species: o["species"], StartsAt: when.UTC()})
	if err != nil {
		a.failDeferred(i, err)
		return
	}
	if e.Visibility == "private" {
		role, err := a.session.GuildRoleCreate(e.GuildID, &discordgo.RoleParams{Name: "Session: " + voiceChannelName(e.Title)})
		if err != nil {
			_, _ = a.closeEvent(context.Background(), e.ID)
			a.editReply(i, "I could not create the private session role. Ensure I have the Manage Roles permission, then try again.")
			return
		}
		e.RoleID = role.ID
		if err := a.setSessionRole(context.Background(), e.ID, role.ID); err != nil {
			_ = a.session.GuildRoleDelete(e.GuildID, role.ID)
			_, _ = a.closeEvent(context.Background(), e.ID)
			a.failDeferred(i, err)
			return
		}
		if err := a.session.GuildMemberRoleAdd(e.GuildID, e.CreatorID, role.ID); err != nil {
			_ = a.session.GuildRoleDelete(e.GuildID, role.ID)
			_, _ = a.closeEvent(context.Background(), e.ID)
			a.editReply(i, "I could not assign the private session role. Ensure I have the Manage Roles permission, then try again.")
			return
		}
		a.editReply(i, "Your private **"+e.Title+"** play session is ready. Share invite code `"+e.InviteCode+"`. Participants join with `/sessions join-private` and the code.")
		return
	}
	a.editReply(i, "Your public play session is listed in `/sessions browse`. Its voice channel will open 15 minutes before it starts.")
}

func (a *app) createSessionVoiceChannel(guildID, categoryID, title string) (*discordgo.Channel, error) {
	return a.session.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{Name: voiceChannelName(title), Type: discordgo.ChannelTypeGuildVoice, ParentID: categoryID})
}

func (a *app) createPrivateSessionVoiceChannel(e event, categoryID string) (*discordgo.Channel, error) {
	permissions := int64(discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak | discordgo.PermissionSendMessages)
	return a.session.GuildChannelCreateComplex(e.GuildID, discordgo.GuildChannelCreateData{Name: voiceChannelName(e.Title), Type: discordgo.ChannelTypeGuildVoice, ParentID: categoryID, PermissionOverwrites: []*discordgo.PermissionOverwrite{
		{ID: e.GuildID, Type: discordgo.PermissionOverwriteTypeRole, Deny: permissions},
		{ID: e.RoleID, Type: discordgo.PermissionOverwriteTypeRole, Allow: permissions},
		{ID: a.session.State.User.ID, Type: discordgo.PermissionOverwriteTypeMember, Allow: permissions},
	}})
}

func voiceChannelName(title string) string {
	name := []rune(strings.TrimSpace(title))
	if len(name) < 2 {
		return "roam"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return string(name)
}

func (a *app) announcePublicSession(e event) error {
	_, err := a.session.ChannelMessageSendComplex(e.VoiceChannelID, &discordgo.MessageSend{Content: eventText(e), Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Join session", Style: discordgo.PrimaryButton, CustomID: "join:" + e.ID},
		discordgo.Button{Label: "Leave session", Style: discordgo.SecondaryButton, CustomID: "leave:" + e.ID},
	}}}})
	return err
}

func parseEventTime(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00"} {
		when, err := time.Parse(layout, value)
		if err == nil {
			return when, nil
		}
	}
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSpace(strings.TrimSuffix(value, " uhr"))
	for german, english := range map[string]string{"heute ": "today ", "morgen ": "tomorrow ", "montag ": "monday ", "dienstag ": "tuesday ", "mittwoch ": "wednesday ", "donnerstag ": "thursday ", "freitag ": "friday ", "samstag ": "saturday ", "sonntag ": "sunday "} {
		if rest, ok := strings.CutPrefix(value, german); ok {
			value = english + rest
			break
		}
	}
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
	for _, layout := range []string{"15:04", "15", "3PM", "3:04PM"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, location), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid clock")
}
func eventText(e event) string {
	species := "Any species"
	server := "No server preference"
	if e.Species != "" {
		species = e.Species
	}
	if e.GameServer != "" {
		server = e.GameServer
	}
	return fmt.Sprintf("**%s**\nHosted by <@%s>\nRoam begins <t:%d:F> (<t:%d:R>)\nEvrima server: %s · Species: %s\nSession ID: `%s`", e.Title, e.CreatorID, e.StartsAt.Unix(), e.StartsAt.Unix(), server, species, e.ID)
}
func (a *app) list(i *discordgo.InteractionCreate) {
	events, err := a.publicEvents(context.Background(), i.GuildID)
	if err != nil {
		a.fail(i, err)
		return
	}
	if len(events) == 0 {
		a.reply(i, text(i, "There are no upcoming public play sessions.", "Es gibt keine kommenden öffentlichen Spielrunden."), true)
		return
	}
	lines := make([]string, 0, len(events))
	options := make([]discordgo.SelectMenuOption, 0, len(events))
	for _, e := range events {
		lines = append(lines, eventText(e))
		options = append(options, discordgo.SelectMenuOption{Label: e.Title, Value: e.ID, Description: e.StartsAt.Format("2006-01-02 15:04 UTC")})
	}
	a.replyWithComponents(i, "**"+text(i, "Upcoming play sessions", "Kommende Spielrunden")+"**\n\n"+strings.Join(lines, "\n\n"), []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.SelectMenu{CustomID: "join-select", Placeholder: text(i, "Choose a play session to join", "Wähle eine Spielrunde zum Beitreten"), Options: options, MaxValues: 1}}}}, true)
}
func (a *app) showJoinEvents(i *discordgo.InteractionCreate) { a.list(i) }
func (a *app) joinPrivate(i *discordgo.InteractionCreate, code string) {
	enabled, err := a.privateSessionsEnabled(context.Background(), i.GuildID)
	if err != nil {
		a.fail(i, err)
		return
	}
	if !enabled {
		a.reply(i, "Private play sessions are not enabled on this server.", true)
		return
	}
	e, err := a.eventByCode(context.Background(), i.GuildID, code)
	if err != nil {
		a.reply(i, "That private invite code is invalid or the session has ended.", true)
		return
	}
	a.joinEvent(i, e.ID)
}
func (a *app) join(i *discordgo.InteractionCreate, id string) {
	e, err := a.eventByID(context.Background(), i.GuildID, id)
	if err != nil {
		a.reply(i, "That play session no longer exists.", true)
		return
	}
	if e.Visibility != "public" {
		a.reply(i, "Use `/sessions join-private` with the invite code for private sessions.", true)
		return
	}
	a.joinEvent(i, e.ID)
}
func (a *app) joinEvent(i *discordgo.InteractionCreate, id string) {
	e, err := a.eventByID(context.Background(), i.GuildID, id)
	if err != nil {
		a.reply(i, "That play session no longer exists.", true)
		return
	}
	joined, err := a.enrollEvent(context.Background(), id, userID(i))
	if err != nil {
		a.fail(i, err)
		return
	}
	if !joined {
		a.reply(i, "You are already in this play session.", true)
		return
	}
	if e.Visibility == "public" && e.VoiceChannelID != "" {
		if _, err := a.session.ChannelMessageSend(e.VoiceChannelID, "<@"+e.CreatorID+"> <@"+userID(i)+"> joined **"+e.Title+"**."); err != nil {
			slog.Error("announce public session participant", "error", err)
		}
	}
	if e.Visibility == "private" {
		if e.RoleID != "" {
			if err := a.session.GuildMemberRoleAdd(e.GuildID, userID(i), e.RoleID); err != nil {
				_, _ = a.leaveEvent(context.Background(), id, userID(i))
				a.reply(i, "I could not grant access to the private session. Please ask an administrator to check my Manage Roles permission.", true)
				return
			}
		}
		if e.VoiceChannelID != "" {
			if _, err := a.session.ChannelMessageSend(e.VoiceChannelID, "<@"+e.CreatorID+"> <@"+userID(i)+"> joined **"+e.Title+"**."); err != nil {
				slog.Error("announce private session participant", "error", err)
			}
		}
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
		a.reply(i, "You are not a participant, or you are the host. Hosts can use `/sessions end`.", true)
		return
	}
	a.reply(i, "You left the play session.", true)
}
func (a *app) showLeaveEvents(i *discordgo.InteractionCreate) {
	events, err := a.participantEvents(context.Background(), i.GuildID, userID(i))
	if err != nil {
		a.fail(i, err)
		return
	}
	if len(events) == 0 {
		a.reply(i, text(i, "You have no play sessions to leave.", "Du hast keine Spielrunden zum Verlassen."), true)
		return
	}
	options := make([]discordgo.SelectMenuOption, 0, len(events))
	for _, e := range events {
		options = append(options, discordgo.SelectMenuOption{Label: e.Title, Value: e.ID, Description: e.StartsAt.Format("2006-01-02 15:04 UTC")})
	}
	a.replyWithComponents(i, text(i, "Choose a play session to leave:", "Wähle eine Spielrunde zum Verlassen:"), []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.SelectMenu{CustomID: "leave-select", Placeholder: text(i, "Select a play session", "Spielrunde auswählen"), Options: options, MaxValues: 1}}}}, true)
}
func (a *app) close(i *discordgo.InteractionCreate, id string) {
	e, err := a.eventByID(context.Background(), i.GuildID, id)
	if err != nil {
		a.reply(i, "That play session no longer exists.", true)
		return
	}
	if e.CreatorID != userID(i) && !canManageSessions(i) {
		a.reply(i, "Only the session host or a server administrator can end an active play session.", true)
		return
	}
	if e.VoiceChannelID != "" {
		if _, err := a.session.ChannelDelete(e.VoiceChannelID); err != nil {
			a.notifyVoiceChannelDeletionFailure(e)
		}
	}
	if e.RoleID != "" {
		if err := a.session.GuildRoleDelete(e.GuildID, e.RoleID); err != nil {
			slog.Error("delete session private role", "session_id", e.ID, "role_id", e.RoleID, "error", err)
		}
	}
	closed, err := a.closeEvent(context.Background(), id)
	if err != nil {
		a.fail(i, err)
		return
	}
	if !closed {
		a.reply(i, "Only the session host can end an active play session.", true)
		return
	}
	a.reply(i, "Your play session has ended and is no longer listed in `/sessions browse`.", true)
}
func (a *app) showCloseEvents(i *discordgo.InteractionCreate) {
	var (
		events []event
		err    error
	)
	if canManageSessions(i) {
		events, err = a.activeEvents(context.Background(), i.GuildID)
	} else {
		events, err = a.creatorEvents(context.Background(), i.GuildID, userID(i))
	}
	if err != nil {
		a.fail(i, err)
		return
	}
	if len(events) == 0 {
		a.reply(i, text(i, "There are no active play sessions to end.", "Es gibt keine aktiven Spielrunden zum Beenden."), true)
		return
	}
	options := make([]discordgo.SelectMenuOption, 0, len(events))
	for _, e := range events {
		options = append(options, discordgo.SelectMenuOption{Label: e.Title, Value: e.ID, Description: e.StartsAt.Format("2006-01-02 15:04 UTC")})
	}
	a.replyWithComponents(i, text(i, "Choose a play session to end:", "Wähle eine Spielrunde zum Beenden:"), []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.SelectMenu{CustomID: "close-select", Placeholder: text(i, "Select a play session", "Spielrunde auswählen"), Options: options, MaxValues: 1}}}}, true)
}
func canManageSessions(i *discordgo.InteractionCreate) bool {
	return i.Member != nil && i.Member.Permissions&(discordgo.PermissionManageServer|discordgo.PermissionAdministrator) != 0
}
func (a *app) botCanManageRoles(guildID string) (bool, error) {
	member, err := a.session.UserGuildMember(guildID)
	if err != nil {
		return false, err
	}
	roles, err := a.session.GuildRoles(guildID)
	if err != nil {
		return false, err
	}
	return memberCanManageRoles(guildID, member, roles), nil
}
func memberCanManageRoles(guildID string, member *discordgo.Member, roles []*discordgo.Role) bool {
	if member == nil {
		return false
	}
	roleIDs := map[string]bool{guildID: true}
	for _, id := range member.Roles {
		roleIDs[id] = true
	}
	var permissions int64
	for _, role := range roles {
		if roleIDs[role.ID] {
			permissions |= role.Permissions
		}
	}
	return permissions&discordgo.PermissionAdministrator != 0 || permissions&discordgo.PermissionManageRoles != 0
}
func (a *app) notifyVoiceChannelDeletionFailure(e event) {
	channelName := voiceChannelName(e.Title)
	if channel, err := a.session.Channel(e.VoiceChannelID); err == nil && channel.Name != "" {
		channelName = channel.Name
	}
	message := voiceChannelDeletionFailureMessage(channelName)
	if _, err := a.session.ChannelMessageSend(e.VoiceChannelID, message); err != nil {
		slog.Error("notify administrators about leftover voice channel", "session_id", e.ID, "voice_channel_id", e.VoiceChannelID, "error", err)
	}
}
func voiceChannelDeletionFailureMessage(channelName string) string {
	return "Server administrators: I could not delete the leftover voice channel **" + channelName + "** because I am missing the Manage Channels permission. The session was removed from my database; please delete this channel manually."
}
func (a *app) handleButton(i *discordgo.InteractionCreate) {
	if i.MessageComponentData().CustomID == "close-select" {
		values := i.MessageComponentData().Values
		if len(values) == 1 {
			a.close(i, values[0])
		}
		return
	}
	if i.MessageComponentData().CustomID == "join-select" {
		values := i.MessageComponentData().Values
		if len(values) == 1 {
			a.join(i, values[0])
		}
		return
	}
	if i.MessageComponentData().CustomID == "leave-select" {
		values := i.MessageComponentData().Values
		if len(values) == 1 {
			a.leave(i, values[0])
		}
		return
	}
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
func (a *app) deferReply(i *discordgo.InteractionCreate, ephemeral bool) bool {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Flags: flags}}); err != nil {
		slog.Error("defer interaction response", "error", err)
		return false
	}
	return true
}
func (a *app) editReply(i *discordgo.InteractionCreate, content string) {
	if _, err := a.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content}); err != nil {
		slog.Error("edit interaction response", "error", err)
	}
}
func (a *app) failDeferred(i *discordgo.InteractionCreate, err error) {
	slog.Error("interaction failed", "error", err)
	a.editReply(i, "Something went wrong. Please try again.")
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
