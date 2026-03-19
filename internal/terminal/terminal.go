// Package terminal configures the process-wide structured logger.
package terminal

import (
	"log/slog"
	"os"

	charmlog "github.com/charmbracelet/log"
)

// Setup initialises charmbracelet/log as the default slog handler.
// When debug is true the level is set to Debug, showing all HTTP requests
// and other verbose output. Otherwise only Info and above are shown.
func Setup(debug bool) {
	level := charmlog.InfoLevel
	if debug {
		level = charmlog.DebugLevel
	}
	logger := charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
		TimeFormat:      "2006/01/02 15:04:05",
		Level:           level,
	})
	slog.SetDefault(slog.New(logger))
}
