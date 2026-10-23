// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "testing"

func TestStudioFlagsReadEnv(t *testing.T) {
	env := map[string]string{
		"RESTITCH_DATABASE_URL":       "sqlite:/data/studio.db",
		"RESTITCH_PUBLIC_URL":         "https://studio.example.com",
		"RESTITCH_BOOTSTRAP_EMAIL":    "owner@example.com",
		"RESTITCH_BOOTSTRAP_PASSWORD": "pw",
	}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	opts, err := parseStudioFlags(nil, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if opts.cfg.DatabaseURL != "sqlite:/data/studio.db" || opts.cfg.PublicURL != "https://studio.example.com" ||
		opts.cfg.BootstrapEmail != "owner@example.com" || opts.port != 3080 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestStudioFlagsRefuseIncompleteSettings(t *testing.T) {
	cases := map[string]map[string]string{
		"no database":        {},
		"email, no password": {"RESTITCH_DATABASE_URL": "sqlite:x.db", "RESTITCH_BOOTSTRAP_EMAIL": "a@b.c"},
		"issuer, no client":  {"RESTITCH_DATABASE_URL": "sqlite:x.db", "RESTITCH_OIDC_ISSUER": "https://login.example.com"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
			if _, err := parseStudioFlags(nil, lookup); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestStudioPublicURLDefaultsToPort(t *testing.T) {
	lookup := func(k string) (string, bool) {
		if k == "RESTITCH_DATABASE_URL" {
			return "sqlite:x.db", true
		}
		return "", false
	}
	opts, err := parseStudioFlags([]string{"-port", "4000"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if opts.cfg.PublicURL != "http://localhost:4000" {
		t.Fatalf("public url = %q", opts.cfg.PublicURL)
	}
}
