package internal

import (
	"os"

	"io"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// provides a logger with optional name as section key
func GetLogger(section string) zerolog.Logger {
	if section == "" {
		return log.Logger
	}
	return log.Logger.With().Str("section", section).Logger()
}

// InitLogging sets up logging for the given level and mode
func InitLogging(level string, mode string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	var w io.Writer
	if mode == "json" {
		w = os.Stderr
	} else {
		w = zerolog.ConsoleWriter{Out: os.Stderr}
		if mode != "plain" {
			log.Logger.Warn().Str("requested", mode).Msg("unknown log mode")
		}
	}
	log.Logger = log.Logger.Output(w)

	l, err := zerolog.ParseLevel(level)
	if err != nil {
		l = zerolog.GlobalLevel()
		log.Logger.Warn().Str("requested", level).Str("fallback", l.String()).Msg("unknown log level")
	}
	log.Logger.Info().Str("loglevel", l.String()).Msg("setting log level")
	zerolog.SetGlobalLevel(l)
}
