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

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

// publishCmd replaces an environment's draft with a file and publishes it,
// the same path the Studio UI takes. CI runs it with an API key.
func publishCmd(args []string) int {
	return runPublish(args, os.LookupEnv, os.Stdout, os.Stderr)
}

func runPublish(args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.SetOutput(stderr)
	studioURL := flags.String("studio-url", "", "Studio URL")
	apiKey := flags.String("api-key", "", "Studio API key of an editor or owner")
	env := flags.String("env", "", "Environment to publish to")
	message := flags.String("m", "Published with restitch publish", "Version message")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := gwconfig.BindEnv(flags, lookup); err != nil {
		fmt.Fprintf(stderr, "publish: %v\n", err)
		return 2
	}
	if *studioURL == "" || *apiKey == "" || *env == "" || flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: restitch publish -env NAME [-m MESSAGE] FILE (with RESTITCH_STUDIO_URL and RESTITCH_API_KEY)")
		return 2
	}
	doc, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "publish: %v\n", err)
		return 1
	}

	c := &publishClient{base: strings.TrimRight(*studioURL, "/"), key: *apiKey, hc: &http.Client{Timeout: 60 * time.Second}}
	envPath := "/api/v1/environments/" + url.PathEscape(*env)
	var imported struct {
		Diff string `json:"diff"`
	}
	if err := c.post(envPath+"/import", map[string]string{"yaml": string(doc)}, &imported); err != nil {
		fmt.Fprintf(stderr, "publish: %v\n", err)
		return 1
	}
	if imported.Diff == "" {
		fmt.Fprintln(stdout, "The file matches the draft.")
	} else {
		fmt.Fprint(stdout, imported.Diff)
	}
	var version struct {
		Number int `json:"number"`
	}
	if err := c.post(envPath+"/publish", map[string]string{"message": *message}, &version); err != nil {
		fmt.Fprintf(stderr, "publish: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Published %s version %d.\n", *env, version.Number)
	return 0
}

type publishClient struct {
	base, key string
	hc        *http.Client
}

func (c *publishClient) post(path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var problem struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(raw, &problem) == nil && problem.Detail != "" {
			return errors.New(problem.Detail)
		}
		return fmt.Errorf("studio returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}
