// Package logging builds the structured logger used across WhatGate binaries.
// It wraps stdlib log/slog (zero external dependency, matching the project's
// distribution-simplicity goal): "text" gives human-readable key=value output
// for interactive use, "json" gives one machine-parseable object per line so an
// operator can ship logs to a collector and filter/aggregate operational and
// security events (rate limits, Sybil isolation, bandwidth trips, ...).
package logging

import (
	"io"
	"log/slog"
)

// New returns a slog.Logger writing to w. format "json" selects the JSON handler;
// any other value (including "text" and "") selects the human-readable text
// handler.
func New(w io.Writer, format string) *slog.Logger {
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(w, nil)
	} else {
		h = slog.NewTextHandler(w, nil)
	}
	return slog.New(h)
}
