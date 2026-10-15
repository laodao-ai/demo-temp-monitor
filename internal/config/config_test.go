package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodURL = "postgres://u:CHANGE_ME@127.0.0.1:7432/db?sslmode=disable"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// writeFile writes content to a temp file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "local.env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPriority(t *testing.T) {
	file := writeFile(t, "TM_DATABASE_URL=file-url\nTM_LISTEN_ADDR=127.0.0.1:1111\nTM_ALARM_THRESHOLD_C=60\n")

	t.Run("env beats file", func(t *testing.T) {
		got, err := Load(env(map[string]string{
			"TM_DATABASE_URL": "env-url", "TM_LISTEN_ADDR": "127.0.0.1:2222", "TM_ALARM_THRESHOLD_C": "70",
		}), file)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "env-url" || got.ListenAddr != "127.0.0.1:2222" || got.AlarmThresholdC != 70 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("file beats default", func(t *testing.T) {
		got, err := Load(env(nil), file)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "file-url" || got.ListenAddr != "127.0.0.1:1111" || got.AlarmThresholdC != 60 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		got, err := Load(env(map[string]string{"TM_DATABASE_URL": goodURL}), filepath.Join(t.TempDir(), "absent.env"))
		if err != nil {
			t.Fatal(err)
		}
		if got.ListenAddr != "127.0.0.1:8080" || got.AlarmThresholdC != 50.0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("empty env var counts as unset", func(t *testing.T) {
		got, err := Load(env(map[string]string{"TM_DATABASE_URL": "", "TM_LISTEN_ADDR": ""}), file)
		if err != nil {
			t.Fatal(err)
		}
		if got.DatabaseURL != "file-url" || got.ListenAddr != "127.0.0.1:1111" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestLoadFileMissingIsFine(t *testing.T) {
	got, err := Load(env(map[string]string{"TM_DATABASE_URL": goodURL}), filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || got.DatabaseURL != goodURL {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLoadMissingDatabaseURL(t *testing.T) {
	_, err := Load(env(nil), filepath.Join(t.TempDir(), "nope.env"))
	if err == nil || !strings.Contains(err.Error(), "TM_DATABASE_URL is not set") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadInvalidThreshold(t *testing.T) {
	for _, v := range []string{"abc", "NaN", "nan", "Inf", "+Inf", "-Inf", "inf", "Infinity", "1e999"} {
		t.Run(v, func(t *testing.T) {
			_, err := Load(env(map[string]string{"TM_DATABASE_URL": goodURL, "TM_ALARM_THRESHOLD_C": v}), "absent.env")
			if err == nil || !strings.Contains(err.Error(), "TM_ALARM_THRESHOLD_C is invalid") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestLoadFileGrammar(t *testing.T) {
	t.Run("comments and blank lines", func(t *testing.T) {
		p := writeFile(t, "# comment\n\n   \n  # indented comment\nTM_DATABASE_URL=u\n")
		got, err := Load(env(nil), p)
		if err != nil || got.DatabaseURL != "u" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("value with = and &", func(t *testing.T) {
		u := "postgres://u:a=b&c@h:7432/d?sslmode=disable&x=y"
		got, err := Load(env(nil), writeFile(t, "TM_DATABASE_URL="+u+"\n"))
		if err != nil || got.DatabaseURL != u {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("keys and values trimmed", func(t *testing.T) {
		got, err := Load(env(nil), writeFile(t, "  TM_DATABASE_URL  =  u  \n"))
		if err != nil || got.DatabaseURL != "u" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("CRLF line endings", func(t *testing.T) {
		got, err := Load(env(nil), writeFile(t, "TM_DATABASE_URL=u\r\nTM_LISTEN_ADDR=127.0.0.1:9\r\n"))
		if err != nil || got.DatabaseURL != "u" || got.ListenAddr != "127.0.0.1:9" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("empty value counts as unset", func(t *testing.T) {
		got, err := Load(env(map[string]string{"TM_DATABASE_URL": goodURL}), writeFile(t, "TM_LISTEN_ADDR=\nTM_ALARM_THRESHOLD_C=\n"))
		if err != nil || got.ListenAddr != "127.0.0.1:8080" || got.AlarmThresholdC != 50 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("no variable expansion", func(t *testing.T) {
		got, err := Load(env(nil), writeFile(t, "TM_DATABASE_URL=$(touch marker)\n"))
		if err != nil || got.DatabaseURL != "$(touch marker)" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("threshold from file validated", func(t *testing.T) {
		_, err := Load(env(nil), writeFile(t, "TM_DATABASE_URL=u\nTM_ALARM_THRESHOLD_C=NaN\n"))
		if err == nil || !strings.Contains(err.Error(), "TM_ALARM_THRESHOLD_C is invalid") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLoadFileErrors(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"no equals", "# c\nTM_DATABASE_URL=u\nSECRETLINE\n", `line 3: missing "="`},
		{"unknown key", "TM_DATABASE_URL=u\nFOO=secretvalue\n", "line 2: unknown key"},
		{"export prefix is unknown key", "export TM_DATABASE_URL=u\n", "line 1: unknown key"},
		{"double quote", "TM_DATABASE_URL=\"postgres://u:pw@h/d\"\n", "line 1: value must not be quoted"},
		{"single quote", "\nTM_LISTEN_ADDR='127.0.0.1:1'\n", "line 2: value must not be quoted"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeFile(t, tc.content)
			_, err := Load(env(nil), p)
			if err == nil {
				t.Fatal("want error")
			}
			msg := err.Error()
			if !strings.Contains(msg, p+" "+tc.want) {
				t.Fatalf("err = %q, want it to contain %q with the file name", msg, p+" "+tc.want)
			}
			for _, leak := range []string{"SECRETLINE", "secretvalue", "pw@h"} {
				if strings.Contains(msg, leak) {
					t.Fatalf("error leaks line content %q: %q", leak, msg)
				}
			}
		})
	}
}

// An unreadable env file reports the errno text after the fixed prefix, and
// nothing else from the OS error (it would repeat the path).
func TestLoadUnreadableFileReportsCause(t *testing.T) {
	dir := t.TempDir() // reading a directory fails with "is a directory"
	_, err := Load(env(nil), dir)
	if err == nil {
		t.Fatal("want error")
	}
	want := dir + ": cannot read file: is a directory"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
