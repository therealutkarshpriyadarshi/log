package logging

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog.Logger
type Logger struct {
	zerolog.Logger
}

// Config holds logger configuration
type Config struct {
	Level  string
	Format string // "json" or "console"
	Output io.Writer
}

// New creates a new logger instance
func New(cfg Config) *Logger {
	var level zerolog.Level
	switch cfg.Level {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "fatal":
		level = zerolog.FatalLevel
	default:
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	var logger zerolog.Logger
	if cfg.Format == "console" {
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(output).With().Timestamp().Logger()
	}

	return &Logger{Logger: logger}
}

// SetGlobal sets the global logger
func SetGlobal(logger *Logger) {
	log.Logger = logger.Logger
}

// Global returns the global logger
func Global() *Logger {
	return &Logger{Logger: log.Logger}
}

// WithComponent creates a child logger with a component field
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With().Str("component", component).Logger(),
	}
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.With().Interface(key, value).Logger(),
	}
}
