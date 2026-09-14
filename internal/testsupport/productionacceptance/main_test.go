package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptanceConfigRejectsUnownedResources(t *testing.T) {
	owner := "spacc_0123456789abcdef"
	root := filepath.Join(t.TempDir(), owner)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".owner"), []byte(owner), 0600); err != nil {
		t.Fatal(err)
	}
	valid := acceptanceConfig{Owner: owner, Root: root, DatabaseURL: "postgres://" + owner + ":test-only@127.0.0.1:55439/" + owner + "?sslmode=disable", Address: "127.0.0.1:18439"}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*acceptanceConfig){
		"query host override": func(c *acceptanceConfig) { c.DatabaseURL += "&host=192.168.31.93" },
		"default database": func(c *acceptanceConfig) {
			c.DatabaseURL = "postgres://semlia:test@127.0.0.1:55439/semlia?sslmode=disable"
		},
		"remote database": func(c *acceptanceConfig) { c.DatabaseURL = "postgres://" + owner + ":test@192.168.31.93:5432/" + owner },
		"default port":    func(c *acceptanceConfig) { c.DatabaseURL = "postgres://" + owner + ":test@127.0.0.1:5432/" + owner },
		"public bind":     func(c *acceptanceConfig) { c.Address = "0.0.0.0:18439" },
		"ambiguous owner": func(c *acceptanceConfig) { c.Owner = "semlia" },
		"wrong root":      func(c *acceptanceConfig) { c.Root = filepath.Dir(root) },
		"unmarked root":   func(c *acceptanceConfig) { c.Root = filepath.Join(t.TempDir(), owner); _ = os.Mkdir(c.Root, 0700) },
		"symlink root": func(c *acceptanceConfig) {
			link := filepath.Join(t.TempDir(), owner)
			if err := os.Symlink(root, link); err != nil {
				t.Fatal(err)
			}
			c.Root = link
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := valid
			change(&c)
			if c.validate() == nil {
				t.Fatal("accepted unsafe configuration")
			}
		})
	}
}
