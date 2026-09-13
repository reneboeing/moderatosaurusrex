package main

import (
	"testing"
	"time"
)

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
