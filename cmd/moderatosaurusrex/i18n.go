package main

import "github.com/bwmarrin/discordgo"

// text uses Discord's interaction locale. Unsupported locales fall back to English.
func text(i *discordgo.InteractionCreate, english, german string) string {
	if i.Locale == discordgo.German {
		return german
	}
	return english
}
