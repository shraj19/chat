package logger

import (
	"log/slog"
	"os"
)

var log *slog.Logger

func Init(env string) {
	if env == "production" {
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	} else {
		// In development, also write to stdout for Docker log collection (Promtail/Loki)
		// Use JSON format for structured log parsing in Grafana
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
}

func InitTest() {
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func Debug(msg string, args ...any) {
	if log != nil {
		log.Debug(msg, args...)
	}
}

func Info(msg string, args ...any) {
	if log != nil {
		log.Info(msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if log != nil {
		log.Warn(msg, args...)
	}
}

func Error(msg string, args ...any) {
	if log != nil {
		log.Error(msg, args...)
	}
}

func Get() *slog.Logger {
	return log
}
