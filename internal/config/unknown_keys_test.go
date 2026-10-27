package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func keyStrings(keys []UnknownKey) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.String())
	}
	return out
}

func TestFindUnknownKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{name: "empty file", yaml: "", want: nil},
		{name: "comment only", yaml: "# nothing here\n", want: nil},
		{name: "known keys only", yaml: "server:\n  port: 1\nproxy:\n  engine: olla\n", want: nil},
		{
			name: "nested path and line",
			yaml: "server:\n  port: 1\nproxy:\n  engine: olla\n  bogus_key: 1\n",
			want: []string{"proxy.bogus_key (line 5)"},
		},
		{
			name: "several unknowns at once, document order",
			yaml: "top_level: 1\nserver:\n  rate_limits:\n    per_endpoint:\n      x: 1\nproxy:\n  retry:\n    enabled: true\n  bogus: 2\n",
			want: []string{"top_level (line 1)", "server.rate_limits.per_endpoint (line 4)", "proxy.retry (line 7)", "proxy.bogus (line 9)"},
		},
		{
			name: "children of an unknown key are not reported separately",
			yaml: "proxy:\n  retry:\n    nested:\n      deeper: 1\n",
			want: []string{"proxy.retry (line 2)"},
		},
		{
			name: "free-form map keys are not unknown",
			yaml: "model_aliases:\n  anything_goes:\n    - a\n    - b\n",
			want: nil,
		},
		{
			name: "struct inside a slice gets an indexed path",
			yaml: "discovery:\n  static:\n    endpoints:\n      - name: a\n        url: http://x\n        typo_field: 1\n      - name: b\n        url: http://y\n        another: 2\n",
			want: []string{"discovery.static.endpoints[0].typo_field (line 6)", "discovery.static.endpoints[1].another (line 9)"},
		},
		{
			name: "free-form map inside a slice element is not unknown",
			yaml: "discovery:\n  static:\n    endpoints:\n      - name: a\n        url: http://x\n        headers:\n          X-Whatever: 1\n",
			want: nil,
		},
		{
			name: "anchor and alias reuse",
			yaml: "x-base: &base\n  engine: olla\nproxy: *base\n",
			want: nil,
		},
		{
			name: "x- extension key is skipped but merged unknowns are still reported",
			yaml: "x-base: &base\n  engine: olla\n  bogus: 1\nproxy:\n  <<: *base\n",
			want: []string{"proxy.bogus (line 3)"},
		},
		{
			name: "x- prefix only exempts top-level keys",
			yaml: "proxy:\n  x-thing: 1\nx-top: 1\nxray: 1\n",
			want: []string{"proxy.x-thing (line 2)", "xray (line 4)"},
		},
		{
			name: "merge key pulls known keys without flagging them",
			yaml: "defaults: &d\n  engine: olla\nproxy:\n  <<: *d\n  profile: auto\n",
			want: []string{"defaults (line 1)"},
		},
		{
			name: "merge key carrying an unknown key",
			yaml: "defaults: &d\n  bogus: 1\nproxy:\n  <<: *d\n  profile: auto\n",
			want: []string{"defaults (line 1)", "proxy.bogus (line 2)"},
		},
		{
			name: "merge sequence",
			yaml: "a: &a\n  bogus_a: 1\nb: &b\n  bogus_b: 1\nproxy:\n  <<: [*a, *b]\n",
			want: []string{"a (line 1)", "b (line 3)", "proxy.bogus_a (line 2)", "proxy.bogus_b (line 4)"},
		},
		{
			name: "alias used as a key resolves to the anchored name",
			yaml: "x-key: &section proxy\n*section:\n  bogus: 1\n",
			want: []string{"proxy.bogus (line 3)"},
		},
		{
			name: "aliased merge tag is a plain unknown key, as yaml.v3 decodes it",
			yaml: "x-key: &key !!merge '<<'\nproxy:\n  *key : {engine: sherpa}\n",
			want: []string{"proxy.<< (line 3)"},
		},
		{name: "merge tag on a non-merge key is an ordinary key", yaml: "!!merge server:\n  port: 41999\n", want: nil},
		{name: "binary-tagged key decodes to the field name", yaml: "!!binary c2VydmVy:\n  port: 41999\n", want: nil},
		{name: "merge tag on an unknown key is still reported", yaml: "!!merge bogus: 1\n", want: []string{"bogus (line 1)"}},
		{name: "scalar document", yaml: "42\n", want: nil},
		{name: "null document", yaml: "~\n", want: nil},
		{name: "unparseable yaml yields nothing", yaml: "server: [unclosed\n", want: nil},
		{name: "wrong shape for a section", yaml: "server: just-a-string\nproxy:\n  - 1\n", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v", r)
					}
				}()
				got = keyStrings(FindUnknownKeys([]byte(tt.yaml)))
			}()
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			// Merge ordering is an implementation detail; compare as sets.
			if !sameSet(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestFindUnknownKeys_OrderIsDocumentOrder(t *testing.T) {
	t.Parallel()
	got := keyStrings(FindUnknownKeys([]byte("b_key: 1\nserver:\n  z: 1\n  a: 2\n")))
	want := []string{"b_key (line 1)", "server.z (line 3)", "server.a (line 4)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoad_RecordsUnknownKeysAndStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 41999\nproxy:\n  bogus_key: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unknown keys must not fail Load: %v", err)
	}
	if cfg.Server.Port != 41999 {
		t.Errorf("known keys must still apply, got port %d", cfg.Server.Port)
	}
	if got := keyStrings(cfg.UnknownKeys); len(got) != 1 || got[0] != "proxy.bogus_key (line 4)" {
		t.Errorf("unexpected unknown keys: %v", got)
	}
}

func TestLoad_NoUnknownKeysWhenClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 41999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnknownKeys) != 0 {
		t.Errorf("expected none, got %v", cfg.UnknownKeys)
	}
}

// The shipped file is allowed to carry dead keys until they are removed, so
// this only checks that detection runs over it without blowing up and that
// every report is well formed - not which keys exist.
func TestFindUnknownKeys_ShippedConfigWellFormed(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../config/config.yaml")
	if err != nil {
		t.Skip("shipped config not found")
	}
	for _, k := range FindUnknownKeys(data) {
		if k.Path == "" || k.Line < 1 || strings.HasPrefix(k.Path, ".") {
			t.Errorf("malformed unknown key: %+v", k)
		}
	}
}

func TestFindUnknownKeys_MergeBombTerminates(t *testing.T) {
	t.Parallel()
	doc := "x-bomb: &bomb\n  <<: [*bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb]\n  bogus: 1\nx-root: &root {proxy: *bomb}\n<<: *root\nproxy: {}\n"
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = FindUnknownKeys([]byte(doc))
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("walker did not terminate on a self-referencing merge")
	}
}

func TestFindUnknownKeys_ReportsAreBounded(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("x-bomb: &bomb\n  <<: [*bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb, *bomb]\n")
	for i := range 1000 {
		fmt.Fprintf(&b, "  f%d: 1\n", i)
	}
	b.WriteString("x-root: &root {proxy: *bomb}\n<<: *root\nproxy: {}\n")
	if got := len(FindUnknownKeys([]byte(b.String()))); got > maxKeyWalkVisits {
		t.Errorf("reports exceeded the visit budget: %d", got)
	}
}
