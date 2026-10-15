package redact_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/laodao-ai/demo-temp-monitor/internal/redact"
)

// Passwords are deliberately not URL-encoded.
var cases = []struct {
	name, pw, fragment string
}{
	{"semicolon-paren", "p(ss;w", "ss;w"},
	{"ampersand", "p&ss", "p&ss"},
	{"bad-percent", "p%zz", "%zz"},
	{"at-and-hash", "p@ss#w", "ss#w"},
}

func connURL(pw string) string {
	return "postgres://user:" + pw + "@127.0.0.1:7432/db"
}

func TestTextRemovesPasswordAndFragments(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := connURL(c.pw)
			in := "dial failed for " + u + "; also saw " + c.pw + " alone"
			got, ok := redact.Text(in, u)
			if !ok {
				t.Fatal("ok = false, want true")
			}
			want := "dial failed for ***; also saw *** alone"
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
			if strings.Contains(got, c.fragment) {
				t.Errorf("fragment %q survived: %q", c.fragment, got)
			}
		})
	}
}

// [impl-review-fix] README tells users to URL-encode special characters, so
// the URL holds "p%40ss%23w" while a driver error may print the decoded "p@ss#w".
func TestTextRemovesDecodedFormOfEncodedPassword(t *testing.T) {
	u := "postgres://user:p%40ss%23w@127.0.0.1:7432/db"
	in := "auth failed for password p@ss#w and raw p%40ss%23w"
	got, ok := redact.Text(in, u)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := "auth failed for password *** and raw ***"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextReportsUnextractablePassword(t *testing.T) {
	for _, u := range []string{
		"postgres://user@127.0.0.1/db",  // no colon before the @
		"postgres://user:@127.0.0.1/db", // empty password
		"not-a-url",
		"",
	} {
		got, ok := redact.Text("failed: "+u, u)
		if ok {
			t.Errorf("url %q: ok = true, want false", u)
		}
		if u != "" && strings.Contains(got, u) {
			t.Errorf("url %q: whole URL survived: %q", u, got)
		}
	}
}

// Per the Handler rule: message and string attributes lose the whole
// connection string, error values (also inside With and groups) lose the
// password and its fragments too.
func TestHandlerRedactsMessageAttrsErrorsAndGroups(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := connURL(c.pw)
			var buf bytes.Buffer
			log := slog.New(redact.Handler(slog.NewTextHandler(&buf, nil), u)).
				With("preset", "x "+u, "preset_err", errors.New("x "+c.pw))
			log.Error("msg "+u,
				"error", errors.New("dial "+u+" bad "+c.pw),
				"plain", u,
				slog.Group("g", "inner", "has "+u, "inner_err", errors.New("has "+c.pw)))
			out := buf.String()
			for _, bad := range []string{c.pw, c.fragment, u} {
				if strings.Contains(out, bad) {
					t.Errorf("output contains %q: %s", bad, out)
				}
			}
			if !strings.Contains(out, "***") {
				t.Errorf("no mask in output: %s", out)
			}
		})
	}
}

func TestHandlerWithholdsErrorTextWhenPasswordUnextractable(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(redact.Handler(slog.NewTextHandler(&buf, nil), "postgres://user@h/db"))
	log.Error("x", "error", errors.New("secret-detail"))
	out := buf.String()
	if strings.Contains(out, "secret-detail") {
		t.Errorf("error text leaked: %s", out)
	}
	if !strings.Contains(out, "error details withheld") {
		t.Errorf("fixed text missing: %s", out)
	}
}

// A short or common password must not rewrite constant text: only error
// values get password replacement.
func TestHandlerKeepsMessageAndStringAttrsWhenPasswordIsCommonWord(t *testing.T) {
	u := connURL("temp")
	var buf bytes.Buffer
	log := slog.New(redact.Handler(slog.NewTextHandler(&buf, nil), u))
	log.Warn("over-temperature alarm",
		"device_id", "temp-01",
		slog.Group("g", "inner", "temp-02"),
		"error", errors.New("auth failed for password temp"))
	out := buf.String()
	for _, want := range []string{"over-temperature alarm", "device_id=temp-01", "g.inner=temp-02"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lost %q: %s", want, out)
		}
	}
	if strings.Contains(out, "password temp") {
		t.Errorf("password survived in error value: %s", out)
	}
	if !strings.Contains(out, "auth failed for password ***") {
		t.Errorf("error value not masked: %s", out)
	}
}

func TestHandlerReplacesWholeConnStringInMessageAndStringAttrs(t *testing.T) {
	u := connURL("temp")
	var buf bytes.Buffer
	log := slog.New(redact.Handler(slog.NewTextHandler(&buf, nil), u))
	log.Error("dial "+u, "detail", "see "+u, slog.Group("g", "inner", u))
	out := buf.String()
	if strings.Contains(out, u) {
		t.Errorf("connection string survived: %s", out)
	}
	if strings.Count(out, "***") != 3 {
		t.Errorf("want 3 masks: %s", out)
	}
}
