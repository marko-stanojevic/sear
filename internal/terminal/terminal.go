// Package terminal configures the process-wide structured logger.
package terminal

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"runtime"

	charmlog "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// Setup initialises charmbracelet/log as the default slog handler.
// When debug is true the level is set to Debug, showing all HTTP requests
// and other verbose output. Otherwise only Info and above are shown.
func Setup(debug bool) {
	level := charmlog.InfoLevel
	if debug {
		level = charmlog.DebugLevel
	}

	// On Windows, charmbracelet may write bare \n which the console interprets
	// as a pure line-feed (cursor down, no column reset). Wrapping stderr with
	// a CRLF translator ensures every line starts at column 0.
	var w io.Writer = os.Stderr
	if runtime.GOOS == "windows" {
		w = &crlfWriter{w: os.Stderr}
	}

	logger := charmlog.NewWithOptions(w, charmlog.Options{
		ReportTimestamp: true,
		TimeFormat:      "2006/01/02 15:04:05",
		Level:           level,
	})

	// Detect the colour profile from the real stderr (not the wrapper) so
	// that wrapping doesn't accidentally disable colours on Windows Terminal.
	logger.SetColorProfile(termenv.NewOutput(os.Stderr).ColorProfile())

	slog.SetDefault(slog.New(logger))
}

// crlfWriter translates bare \n to \r\n for Windows consoles that do not
// perform this translation automatically.
type crlfWriter struct{ w io.Writer }

func (c *crlfWriter) Write(p []byte) (n int, err error) {
	// Normalise any existing \r\n to \n first, then expand all \n to \r\n.
	b := bytes.ReplaceAll(p, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	_, err = c.w.Write(b)
	return len(p), err
}
