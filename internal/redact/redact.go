// Package redact removes database connection strings and passwords from text
// and from log records, so driver errors can be logged safely.
package redact

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
)

const mask = "***"

// withheld replaces an error's text when the password cannot be extracted:
// the text could then contain an unmasked password fragment.
const withheld = "error details withheld"

// password returns the raw substring between the first ":" after the scheme
// and the last "@" of connURL. ok is false when it cannot be extracted.
func password(connURL string) (string, bool) {
	_, rest, found := strings.Cut(connURL, "://")
	if !found {
		return "", false
	}
	colon := strings.Index(rest, ":")
	at := strings.LastIndex(rest, "@")
	if colon < 0 || at < 0 || colon >= at {
		return "", false
	}
	pw := rest[colon+1 : at]
	if pw == "" {
		return "", false
	}
	return pw, true
}

// Text replaces the whole connection string, then the password, with "***".
// ok is false when the password cannot be extracted from connURL; the caller
// should then print a fixed message instead of the text.
func Text(text, connURL string) (out string, ok bool) {
	if connURL == "" {
		return text, false
	}
	out = strings.ReplaceAll(text, connURL, mask)
	pw, ok := password(connURL)
	if !ok {
		return out, false
	}
	out = strings.ReplaceAll(out, pw, mask)
	// [impl-review-fix] The URL holds the URL-encoded password but a driver error
	// may print the decoded one; mask the decoded form too.
	if decoded, err := url.PathUnescape(pw); err == nil && decoded != "" && decoded != pw {
		out = strings.ReplaceAll(out, decoded, mask)
	}
	return out, true
}

// Handler wraps inner so every record's message and string attributes
// (including error values and group members) pass through Text.
func Handler(inner slog.Handler, connURL string) slog.Handler {
	_, ok := password(connURL)
	return &handler{inner: inner, connURL: connURL, pwOK: ok}
}

type handler struct {
	inner   slog.Handler
	connURL string
	pwOK    bool
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	msg, _ := Text(r.Message, h.connURL)
	nr := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.attr(a))
		return true
	})
	return h.inner.Handle(ctx, nr)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		cleaned[i] = h.attr(a)
	}
	return &handler{inner: h.inner.WithAttrs(cleaned), connURL: h.connURL, pwOK: h.pwOK}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{inner: h.inner.WithGroup(name), connURL: h.connURL, pwOK: h.pwOK}
}

// attr handles strings, groups and error values only; other KindAny values
// pass through unchanged.
func (h *handler) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s, _ := Text(v.String(), h.connURL)
		return slog.String(a.Key, s)
	case slog.KindGroup:
		members := v.Group()
		cleaned := make([]any, len(members))
		for i, m := range members {
			cleaned[i] = h.attr(m)
		}
		return slog.Group(a.Key, cleaned...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			if !h.pwOK {
				return slog.String(a.Key, withheld)
			}
			s, _ := Text(err.Error(), h.connURL)
			return slog.String(a.Key, s)
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}
