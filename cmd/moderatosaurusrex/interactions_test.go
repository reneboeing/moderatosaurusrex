package main

import (
	"testing"
	"time"
)

func TestParseEventTime(t *testing.T) {
	for _, input := range []string{"2026-09-13T20:43+02:00", "2026-09-13T20:43:00+02:00"} {
		got, err := parseEventTime(input)
		if err != nil {
			t.Fatalf("parseEventTime(%q): %v", input, err)
		}
		if got.UTC().Format(time.RFC3339) != "2026-09-13T18:43:00Z" {
			t.Fatalf("parseEventTime(%q) returned unexpected time %s", input, got)
		}
	}
}
