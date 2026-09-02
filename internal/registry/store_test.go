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

package registry

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

const sampleYAML = `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test-comp:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`

func sampleYAMLWithComposition(compName string) string {
	return sampleYAMLWithUpstreamAndComposition("mock", compName)
}

func sampleYAMLWithUpstreamAndComposition(upstreamName, compName string) string {
	return `
upstreams:
  ` + upstreamName + `:
    url: "http://localhost:8081"
compositions:
  ` + compName + `:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: ` + upstreamName + `
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`
}

func TestStore_CreateAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	author := "nate"
	changeMsg := "Initial config"
	input := CreateConfigInput{
		Name:          "user-service",
		Description:   "User service compositions",
		Tags:          []string{"team-platform", "v1"},
		YAMLContent:   sampleYAML,
		Author:        &author,
		ChangeMessage: &changeMsg,
	}

	created, err := s.CreateConfig(ctx, input)
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Name != input.Name {
		t.Errorf("Name = %q, want %q", created.Name, input.Name)
	}
	if created.Description != input.Description {
		t.Errorf("Description = %q, want %q", created.Description, input.Description)
	}
	if len(created.Tags) != 2 || created.Tags[0] != "team-platform" || created.Tags[1] != "v1" {
		t.Errorf("Tags = %v, want %v", created.Tags, input.Tags)
	}
	if created.YAMLContent != sampleYAML {
		t.Errorf("YAMLContent mismatch")
	}
	if created.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", created.VersionNumber)
	}
	if created.Author == nil || *created.Author != author {
		t.Errorf("Author = %v, want %q", created.Author, author)
	}
	if created.ChangeMessage == nil || *created.ChangeMessage != changeMsg {
		t.Errorf("ChangeMessage = %v, want %q", created.ChangeMessage, changeMsg)
	}
	if created.ActiveVersionID == nil {
		t.Fatal("expected ActiveVersionID to be set")
	}
	if created.ActiveVersion == nil || *created.ActiveVersion != 1 {
		t.Errorf("ActiveVersion = %v, want 1", created.ActiveVersion)
	}

	got, err := s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got == nil {
		t.Fatal("GetConfig returned nil")
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != created.Name {
		t.Errorf("Name = %q, want %q", got.Name, created.Name)
	}
	if got.YAMLContent != sampleYAML {
		t.Errorf("YAMLContent mismatch on Get")
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 entries", got.Tags)
	}
}

func TestStore_ListConfigs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	names := []string{"config-a", "config-b", "config-c"}
	for _, n := range names {
		_, err := s.CreateConfig(ctx, CreateConfigInput{
			Name:        n,
			YAMLContent: sampleYAML,
			Tags:        []string{},
		})
		if err != nil {
			t.Fatalf("CreateConfig(%s): %v", n, err)
		}
	}

	page1, info1, err := s.ListConfigs(ctx, ListConfigsParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListConfigs page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	if !info1.HasMore {
		t.Error("expected HasMore=true")
	}
	if info1.NextCursor == nil {
		t.Fatal("expected NextCursor to be set")
	}

	page2, info2, err := s.ListConfigs(ctx, ListConfigsParams{Limit: 2, Cursor: *info1.NextCursor})
	if err != nil {
		t.Fatalf("ListConfigs page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}
	if info2.HasMore {
		t.Error("expected HasMore=false on last page")
	}

	seen := map[string]bool{}
	for _, c := range page1 {
		seen[c.ID] = true
	}
	for _, c := range page2 {
		if seen[c.ID] {
			t.Errorf("config %s appeared in both pages", c.ID)
		}
	}
	if len(page1)+len(page2) != 3 {
		t.Errorf("total configs across pages = %d, want 3", len(page1)+len(page2))
	}
}

func TestStore_UpdateContent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc",
		YAMLContent: sampleYAML,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	newYAML := sampleYAMLWithComposition("test-comp-v2")
	updated, err := s.UpdateConfigContent(ctx, created.ID, UpdateConfigInput{
		YAMLContent: newYAML,
	})
	if err != nil {
		t.Fatalf("UpdateConfigContent: %v", err)
	}
	if updated.VersionNumber != 2 {
		t.Errorf("VersionNumber = %d, want 2", updated.VersionNumber)
	}
	if updated.YAMLContent != newYAML {
		t.Errorf("YAMLContent mismatch after update")
	}

	got, err := s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.VersionNumber != 2 {
		t.Errorf("GetConfig VersionNumber = %d, want 2", got.VersionNumber)
	}
	if got.YAMLContent != newYAML {
		t.Errorf("GetConfig YAMLContent mismatch")
	}
}

func TestStore_UpdateMetadata(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc",
		YAMLContent: sampleYAML,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	newName := "svc-renamed"
	updated, err := s.UpdateConfigMetadata(ctx, created.ID, UpdateConfigMetadataInput{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateConfigMetadata: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Name = %q, want %q", updated.Name, newName)
	}

	got, err := s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.Name != newName {
		t.Errorf("GetConfig Name = %q, want %q", got.Name, newName)
	}
	if got.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1 (metadata update should not create version)", got.VersionNumber)
	}

	versions, err := s.ListVersions(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("len(versions) = %d, want 1", len(versions))
	}
}

func TestStore_Delete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc",
		YAMLContent: sampleYAML,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	if err := s.DeleteConfig(ctx, created.ID); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}

	got, err := s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig after delete: %v", err)
	}
	if got != nil {
		t.Errorf("GetConfig after delete = %v, want nil", got)
	}

	versions, err := s.ListVersions(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListVersions after delete: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("len(versions) after delete = %d, want 0 (CASCADE)", len(versions))
	}
}

func TestStore_Versions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc",
		YAMLContent: sampleYAML,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	if _, err := s.UpdateConfigContent(ctx, created.ID, UpdateConfigInput{
		YAMLContent: sampleYAMLWithComposition("v2"),
	}); err != nil {
		t.Fatalf("UpdateConfigContent v2: %v", err)
	}
	if _, err := s.UpdateConfigContent(ctx, created.ID, UpdateConfigInput{
		YAMLContent: sampleYAMLWithComposition("v3"),
	}); err != nil {
		t.Fatalf("UpdateConfigContent v3: %v", err)
	}

	versions, err := s.ListVersions(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("len(versions) = %d, want 3", len(versions))
	}
	// newest first
	if versions[0].VersionNumber != 3 || versions[1].VersionNumber != 2 || versions[2].VersionNumber != 1 {
		t.Errorf("version order = %d,%d,%d, want 3,2,1", versions[0].VersionNumber, versions[1].VersionNumber, versions[2].VersionNumber)
	}

	v1, err := s.GetVersion(ctx, created.ID, 1)
	if err != nil {
		t.Fatalf("GetVersion(1): %v", err)
	}
	if v1 == nil || v1.YAMLContent != sampleYAML {
		t.Errorf("GetVersion(1) content mismatch")
	}
}

func TestStore_SetActiveVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc",
		YAMLContent: sampleYAML,
	})
	if err != nil {
		t.Fatalf("CreateConfig: %v", err)
	}

	if _, err := s.UpdateConfigContent(ctx, created.ID, UpdateConfigInput{
		YAMLContent: sampleYAMLWithComposition("v2"),
	}); err != nil {
		t.Fatalf("UpdateConfigContent: %v", err)
	}

	got, err := s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.VersionNumber != 2 {
		t.Fatalf("expected active version 2 before rollback, got %d", got.VersionNumber)
	}

	if err := s.SetActiveVersion(ctx, created.ID, 1); err != nil {
		t.Fatalf("SetActiveVersion: %v", err)
	}

	got, err = s.GetConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetConfig after rollback: %v", err)
	}
	if got.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", got.VersionNumber)
	}
	if got.YAMLContent != sampleYAML {
		t.Errorf("YAMLContent mismatch after rollback")
	}
}

func TestStore_BundleConfig(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc-a",
		YAMLContent: sampleYAMLWithUpstreamAndComposition("mock-a", "comp-a"),
	}); err != nil {
		t.Fatalf("CreateConfig a: %v", err)
	}
	if _, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc-b",
		YAMLContent: sampleYAMLWithUpstreamAndComposition("mock-b", "comp-b"),
	}); err != nil {
		t.Fatalf("CreateConfig b: %v", err)
	}

	bundle, err := s.GetBundledConfig(ctx)
	if err != nil {
		t.Fatalf("GetBundledConfig: %v", err)
	}
	if bundle.ETag == "" {
		t.Error("expected non-empty ETag")
	}
	if bundle.CompositionCount != 2 {
		t.Errorf("CompositionCount = %d, want 2", bundle.CompositionCount)
	}
	if !strings.Contains(bundle.YAMLContent, "comp-a") {
		t.Errorf("bundle YAML missing comp-a: %s", bundle.YAMLContent)
	}
	if !strings.Contains(bundle.YAMLContent, "comp-b") {
		t.Errorf("bundle YAML missing comp-b: %s", bundle.YAMLContent)
	}
	if len(bundle.CompositionNames) != 2 {
		t.Errorf("CompositionNames = %v, want 2 entries", bundle.CompositionNames)
	}
}

func TestStore_BundleNameCollision(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc-a",
		YAMLContent: sampleYAMLWithUpstreamAndComposition("mock-a", "shared-name"),
	}); err != nil {
		t.Fatalf("CreateConfig a: %v", err)
	}
	if _, err := s.CreateConfig(ctx, CreateConfigInput{
		Name:        "svc-b",
		YAMLContent: sampleYAMLWithUpstreamAndComposition("mock-b", "shared-name"),
	}); err != nil {
		t.Fatalf("CreateConfig b: %v", err)
	}

	_, err := s.GetBundledConfig(ctx)
	if err == nil {
		t.Fatal("expected error on composition name collision, got nil")
	}
}
