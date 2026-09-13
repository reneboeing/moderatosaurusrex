package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type event struct {
	ID, GuildID, CreatorID, Title, Visibility, InviteCode, GameServer, Species, VoiceChannelID, RoleID string
	StartsAt                                                                                           time.Time
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS guild_settings (guild_id TEXT PRIMARY KEY, channel_id TEXT NOT NULL);
ALTER TABLE guild_settings ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';
CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY,guild_id TEXT NOT NULL,creator_id TEXT NOT NULL,title TEXT NOT NULL,visibility TEXT NOT NULL CHECK (visibility IN ('public','private')),invite_code TEXT UNIQUE,game_server TEXT NOT NULL,species TEXT NOT NULL DEFAULT '',voice_channel_id TEXT NOT NULL DEFAULT '',starts_at TIMESTAMPTZ NOT NULL,reminder_sent BOOLEAN NOT NULL DEFAULT FALSE,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
ALTER TABLE events ADD COLUMN IF NOT EXISTS voice_channel_id TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN IF NOT EXISTS role_id TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS event_participants (event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,user_id TEXT NOT NULL,is_creator BOOLEAN NOT NULL DEFAULT FALSE,PRIMARY KEY(event_id,user_id));
CREATE INDEX IF NOT EXISTS events_active_idx ON events(guild_id,starts_at);`)
	if err == nil {
		_, err = pool.Exec(ctx, `UPDATE events SET invite_code=NULL WHERE invite_code=''`)
	}
	return err
}
func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (a *app) setSessionCategory(ctx context.Context, guildID, categoryID string) error {
	_, err := a.pool.Exec(ctx, `INSERT INTO guild_settings(guild_id,channel_id) VALUES($1,$2) ON CONFLICT(guild_id) DO UPDATE SET channel_id=EXCLUDED.channel_id`, guildID, categoryID)
	return err
}
func (a *app) setTimezone(ctx context.Context, guildID, timezone string) error {
	_, err := a.pool.Exec(ctx, `INSERT INTO guild_settings(guild_id,channel_id,timezone) VALUES($1,'',$2) ON CONFLICT(guild_id) DO UPDATE SET timezone=EXCLUDED.timezone`, guildID, timezone)
	return err
}
func (a *app) eventTimezone(ctx context.Context, guildID string) (string, error) {
	var timezone string
	err := a.pool.QueryRow(ctx, `SELECT timezone FROM guild_settings WHERE guild_id=$1`, guildID).Scan(&timezone)
	return timezone, err
}
func (a *app) sessionCategory(ctx context.Context, guildID string) (string, error) {
	var id string
	err := a.pool.QueryRow(ctx, `SELECT channel_id FROM guild_settings WHERE guild_id=$1`, guildID).Scan(&id)
	return id, err
}
func (a *app) createEvent(ctx context.Context, e event) (event, error) {
	id, err := randomHex(8)
	if err != nil {
		return event{}, err
	}
	e.ID = id
	if e.Visibility == "private" {
		code, err := randomHex(4)
		if err != nil {
			return event{}, err
		}
		e.InviteCode = "REX-" + code
	}
	inviteCode := databaseInviteCode(e.Visibility, e.InviteCode)
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return event{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO events(id,guild_id,creator_id,title,visibility,invite_code,game_server,species,voice_channel_id,role_id,starts_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, e.ID, e.GuildID, e.CreatorID, e.Title, e.Visibility, inviteCode, e.GameServer, e.Species, e.VoiceChannelID, e.RoleID, e.StartsAt)
	if err != nil {
		return event{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO event_participants(event_id,user_id,is_creator) VALUES($1,$2,true)`, e.ID, e.CreatorID)
	if err != nil {
		return event{}, err
	}
	return e, tx.Commit(ctx)
}

func databaseInviteCode(visibility, inviteCode string) any {
	if visibility == "private" {
		return inviteCode
	}
	return nil
}
func scanEvent(row pgx.Row) (event, error) {
	var e event
	err := row.Scan(&e.ID, &e.GuildID, &e.CreatorID, &e.Title, &e.Visibility, &e.InviteCode, &e.GameServer, &e.Species, &e.VoiceChannelID, &e.RoleID, &e.StartsAt)
	return e, err
}
func (a *app) eventByID(ctx context.Context, guildID, id string) (event, error) {
	return scanEvent(a.pool.QueryRow(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE guild_id=$1 AND id=$2`, guildID, id))
}
func (a *app) eventByCode(ctx context.Context, guildID, code string) (event, error) {
	return scanEvent(a.pool.QueryRow(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE guild_id=$1 AND invite_code=$2`, guildID, code))
}
func (a *app) publicEvents(ctx context.Context, guildID string) ([]event, error) {
	rows, err := a.pool.Query(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE guild_id=$1 AND visibility='public' ORDER BY starts_at LIMIT 10`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) creatorEvents(ctx context.Context, guildID, creatorID string) ([]event, error) {
	rows, err := a.pool.Query(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE guild_id=$1 AND creator_id=$2 ORDER BY starts_at LIMIT 25`, guildID, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) activeEvents(ctx context.Context, guildID string) ([]event, error) {
	rows, err := a.pool.Query(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE guild_id=$1 ORDER BY starts_at LIMIT 25`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) participantEvents(ctx context.Context, guildID, userID string) ([]event, error) {
	rows, err := a.pool.Query(ctx, `SELECT e.id,e.guild_id,e.creator_id,e.title,e.visibility,COALESCE(e.invite_code,''),e.game_server,e.species,e.voice_channel_id,e.role_id,e.starts_at FROM events e JOIN event_participants p ON p.event_id=e.id WHERE e.guild_id=$1 AND p.user_id=$2 AND p.is_creator=false ORDER BY e.starts_at LIMIT 25`, guildID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) enrollEvent(ctx context.Context, eventID, userID string) (bool, error) {
	var joined bool
	err := a.pool.QueryRow(ctx, `INSERT INTO event_participants(event_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING true`, eventID, userID).Scan(&joined)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return joined, err
}
func (a *app) leaveEvent(ctx context.Context, eventID, userID string) (bool, error) {
	tag, err := a.pool.Exec(ctx, `DELETE FROM event_participants WHERE event_id=$1 AND user_id=$2 AND is_creator=false`, eventID, userID)
	return tag.RowsAffected() > 0, err
}
func (a *app) closeEvent(ctx context.Context, eventID string) (bool, error) {
	tag, err := a.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, eventID)
	return tag.RowsAffected() > 0, err
}
func (a *app) setVoiceChannel(ctx context.Context, eventID, channelID string) error {
	_, err := a.pool.Exec(ctx, `UPDATE events SET voice_channel_id=$2 WHERE id=$1`, eventID, channelID)
	return err
}
func (a *app) setSessionRole(ctx context.Context, eventID, roleID string) error {
	_, err := a.pool.Exec(ctx, `UPDATE events SET role_id=$2 WHERE id=$1`, eventID, roleID)
	return err
}
func (a *app) retryReminder(ctx context.Context, eventID string) error {
	_, err := a.pool.Exec(ctx, `UPDATE events SET reminder_sent=false WHERE id=$1`, eventID)
	return err
}
func (a *app) participantIDs(ctx context.Context, eventID string) ([]string, error) {
	rows, err := a.pool.Query(ctx, `SELECT user_id FROM event_participants WHERE event_id=$1`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (a *app) dueReminders(ctx context.Context) ([]event, error) {
	rows, err := a.pool.Query(ctx, `UPDATE events SET reminder_sent=true WHERE id IN (SELECT id FROM events WHERE reminder_sent=false AND starts_at BETWEEN NOW()-INTERVAL '5 minutes' AND NOW()+INTERVAL '15 minutes' FOR UPDATE SKIP LOCKED) RETURNING id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) archiveExpired(ctx context.Context) ([]event, error) {
	rows, err := a.pool.Query(ctx, `SELECT id,guild_id,creator_id,title,visibility,COALESCE(invite_code,''),game_server,species,voice_channel_id,role_id,starts_at FROM events WHERE starts_at+INTERVAL '8 hours'<=NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (a *app) archiveEvent(ctx context.Context, eventID string) error {
	_, err := a.pool.Exec(ctx, `DELETE FROM events WHERE id=$1`, eventID)
	return err
}
