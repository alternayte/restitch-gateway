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

func TestGatewayCmd_MutualExclusivity(t *testing.T) {
	code := gatewayCmd([]string{"-config", "x.yaml", "-registry-url", "http://example.com"})
	if code != 2 {
		t.Errorf("expected exit 2 for mutual exclusivity, got %d", code)
	}
}

func TestGatewayFlagOverEnvOverDefault(t *testing.T) {
	env := map[string]string{
		"RESTITCH_PORT":      "9001",
		"RESTITCH_LOG_LEVEL": "debug",
		"RESTITCH_CONFIG":    "/etc/restitch/artifact.yaml",
	}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	opts, err := parseGatewayFlags([]string{"-port", "9002"}, lookup)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.port != 9002 {
		t.Errorf("port: flag must beat env, got %d", opts.port)
	}
	if opts.logLevel != "debug" {
		t.Errorf("log-level: env must beat default, got %q", opts.logLevel)
	}
	if opts.configFile != "/etc/restitch/artifact.yaml" {
		t.Errorf("config: RESTITCH_CONFIG must set the file, got %q", opts.configFile)
	}
	if opts.logFormat != "json" {
		t.Errorf("log-format: default must apply, got %q", opts.logFormat)
	}
	if !opts.set["port"] || !opts.set["log-level"] || opts.set["log-format"] {
		t.Errorf("set must hold flags given by flag or env only, got %v", opts.set)
	}
}

func TestGatewayEnvRejectsInvalidValue(t *testing.T) {
	lookup := func(k string) (string, bool) {
		if k == "RESTITCH_PORT" {
			return "eighty", true
		}
		return "", false
	}
	if _, err := parseGatewayFlags(nil, lookup); err == nil {
		t.Fatal("expected an error for RESTITCH_PORT=eighty")
	}
}
