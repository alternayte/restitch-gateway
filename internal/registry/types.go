// Package registry provides domain types and storage operations for the config registry.
package registry

import "time"

// Config represents a configuration entry in the registry.
type Config struct {
	ID              string    `json:"id"`               // ULID
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Tags            []string  `json:"tags"`
	ActiveVersionID *int64    `json:"active_version_id,omitempty"`
	ActiveVersion   *int      `json:"active_version,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ConfigVersion represents an immutable version of a configuration.
type ConfigVersion struct {
	ID            int64     `json:"id"`
	ConfigID      string    `json:"config_id"`
	VersionNumber int       `json:"version_number"`    // sequential: 1, 2, 3...
	YAMLContent   string    `json:"yaml_content"`
	Author        *string   `json:"author,omitempty"`
	ChangeMessage *string   `json:"change_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ConfigWithContent represents a configuration along with its active version content.
type ConfigWithContent struct {
	Config
	YAMLContent   string  `json:"yaml_content"`
	VersionNumber int     `json:"version_number"`
	Author        *string `json:"author,omitempty"`
	ChangeMessage *string `json:"change_message,omitempty"`
}

// BundledConfig represents a merged configuration from all active versions.
type BundledConfig struct {
	YAMLContent      string   `json:"yaml_content"`
	ETag             string   `json:"etag"`
	CompositionCount int      `json:"composition_count"`
	CompositionNames []string `json:"composition_names"`
}

// CreateConfigInput is the request payload for creating a new configuration.
type CreateConfigInput struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	YAMLContent   string   `json:"yaml_content"`
	Author        *string  `json:"author,omitempty"`
	ChangeMessage *string  `json:"change_message,omitempty"`
}

// UpdateConfigInput is the request payload for updating configuration content.
type UpdateConfigInput struct {
	YAMLContent   string  `json:"yaml_content"`
	Author        *string `json:"author,omitempty"`
	ChangeMessage *string `json:"change_message,omitempty"`
}

// UpdateConfigMetadataInput is the request payload for updating configuration metadata.
type UpdateConfigMetadataInput struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ListConfigsParams contains parameters for listing configurations.
type ListConfigsParams struct {
	Cursor string
	Limit  int
}

// PageInfo contains pagination information.
type PageInfo struct {
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

// ValidationResult represents the result of validating a configuration.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError represents a single validation error with position information.
type ValidationError struct {
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}
