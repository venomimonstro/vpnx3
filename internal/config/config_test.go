package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		if got := parseLogLevel(tt.input); got != tt.want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("VPNX3_TEST_DURATION", "7s")
	if got := duration("VPNX3_TEST_DURATION", time.Second); got != 7*time.Second {
		t.Fatalf("duration = %v, want 7s", got)
	}

	t.Setenv("VPNX3_TEST_DURATION", "12")
	if got := duration("VPNX3_TEST_DURATION", time.Second); got != 12*time.Second {
		t.Fatalf("numeric duration = %v, want 12s", got)
	}

	t.Setenv("VPNX3_TEST_DURATION", "invalid")
	if got := duration("VPNX3_TEST_DURATION", 3*time.Second); got != 3*time.Second {
		t.Fatalf("invalid duration = %v, want fallback 3s", got)
	}
}
