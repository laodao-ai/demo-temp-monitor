package testdb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/laodao-ai/demo-temp-monitor/internal/config"
)

func TestFindRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	got, err := findRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Compare resolved paths: macOS temp dirs sit behind a /var -> /private/var link.
	want, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("findRoot = %q, want %q", gotResolved, want)
	}
}

func TestConfigFailure(t *testing.T) {
	path := "/abs/repo/config/local.env"
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"missing url",
			config.ErrNoURL,
			"TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in",
		},
		{
			"invalid threshold",
			config.ErrThresholdInvalid,
			"TM_ALARM_THRESHOLD_C is invalid; must be a finite number, such as 50.0",
		},
		{
			"missing equals shows the fixed file name",
			fmt.Errorf("%s line 3: %w", path, config.ErrMissingEquals),
			`config/local.env line 3: missing "="; use KEY=VALUE`,
		},
		{
			"unknown key",
			fmt.Errorf("%s line 2: %w", path, config.ErrUnknownKey),
			"config/local.env line 2: unknown key; allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C",
		},
		{
			"quoted value",
			fmt.Errorf("%s line 5: %w", path, config.ErrQuotedValue),
			"config/local.env line 5: value must not be quoted; remove the quotes",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := configFailure(c.err, path); got != c.want {
				t.Fatalf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
