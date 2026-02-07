package logger

import (
	"log/slog"
	"testing"
)

func TestSetup_DefaultLevel(t *testing.T) {
	Setup("info")
	if !slog.Default().Enabled(nil, slog.LevelInfo) {
		t.Error("expected info level to be enabled")
	}
}

func TestSetup_DebugLevel(t *testing.T) {
	Setup("debug")
	if !slog.Default().Enabled(nil, slog.LevelDebug) {
		t.Error("expected debug level to be enabled")
	}
}

func TestSetup_WarnLevel(t *testing.T) {
	Setup("warn")
	if !slog.Default().Enabled(nil, slog.LevelWarn) {
		t.Error("expected warn level to be enabled")
	}
	if slog.Default().Enabled(nil, slog.LevelInfo) {
		t.Error("expected info level to be disabled at warn level")
	}
}

func TestSetup_ErrorLevel(t *testing.T) {
	Setup("error")
	if !slog.Default().Enabled(nil, slog.LevelError) {
		t.Error("expected error level to be enabled")
	}
	if slog.Default().Enabled(nil, slog.LevelWarn) {
		t.Error("expected warn level to be disabled at error level")
	}
}

func TestSetup_UnknownDefaultsToInfo(t *testing.T) {
	Setup("unknown")
	if !slog.Default().Enabled(nil, slog.LevelInfo) {
		t.Error("expected info level to be enabled for unknown input")
	}
}

func TestSetup_CaseInsensitive(t *testing.T) {
	Setup("DEBUG")
	if !slog.Default().Enabled(nil, slog.LevelDebug) {
		t.Error("expected debug level to be enabled with uppercase input")
	}
}
