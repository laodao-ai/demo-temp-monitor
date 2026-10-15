package testdb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
		err  string
		want string
	}{
		{
			"missing url",
			"TM_DATABASE_URL is not set",
			"TM_DATABASE_URL is not set; copy config/example.env to config/local.env and fill it in",
		},
		{
			"missing equals shows the fixed file name",
			path + ` line 3: missing "="`,
			`config/local.env line 3: missing "="; use KEY=VALUE`,
		},
		{
			"unknown key",
			path + " line 2: unknown key",
			"config/local.env line 2: unknown key; allowed keys: TM_DATABASE_URL, TM_LISTEN_ADDR, TM_ALARM_THRESHOLD_C",
		},
		{
			"quoted value",
			path + " line 5: value must not be quoted",
			"config/local.env line 5: value must not be quoted; remove the quotes",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := configFailure(errors.New(c.err), path); got != c.want {
				t.Fatalf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
