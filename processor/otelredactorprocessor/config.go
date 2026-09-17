// Package otelredactorprocessor implements policy-based, recursive trace redaction.
package otelredactorprocessor

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Config controls a traces-only redaction policy. Construct it with DefaultConfig
// before overriding fields so omitted safety limits retain their defaults.
type Config struct {
	DeleteKeys      []string `mapstructure:"delete_keys"`
	Patterns        []string `mapstructure:"patterns"`
	Replacement     string   `mapstructure:"replacement"`
	ScrubTextFields bool     `mapstructure:"scrub_text_fields"`
	MaxDepth        int      `mapstructure:"max_depth"`
	MaxStringBytes  int      `mapstructure:"max_string_bytes"`
}

// DefaultConfig deliberately has no implicit sensitive-data detection policy.
// At least one delete key or pattern must be configured before use.
func DefaultConfig() *Config {
	return &Config{
		Replacement: "[REDACTED]", ScrubTextFields: true,
		MaxDepth: 32, MaxStringBytes: 1 << 20,
	}
}

// Validate rejects unusable or excessive policies without printing their contents.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("otelredactor: config is nil")
	}
	if len(c.DeleteKeys)+len(c.Patterns) == 0 {
		return errors.New("otelredactor: at least one delete_keys entry or pattern is required")
	}
	if len(c.DeleteKeys) > 1024 || len(c.Patterns) > 128 {
		return errors.New("otelredactor: at most 1024 delete keys and 128 patterns are allowed")
	}
	if c.MaxDepth < 1 || c.MaxDepth > 128 {
		return errors.New("otelredactor: max_depth must be between 1 and 128")
	}
	if c.MaxStringBytes < 1 || c.MaxStringBytes > 16<<20 {
		return errors.New("otelredactor: max_string_bytes must be between 1 and 16777216")
	}
	if strings.TrimSpace(c.Replacement) == "" || len(c.Replacement) > c.MaxStringBytes {
		return errors.New("otelredactor: replacement must be nonblank and within max_string_bytes")
	}
	seen := make(map[string]struct{}, len(c.DeleteKeys))
	for i, key := range c.DeleteKeys {
		if strings.TrimSpace(key) == "" || len(key) > 4096 {
			return fmt.Errorf("otelredactor: delete_keys[%d] must be nonblank and at most 4096 bytes", i)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("otelredactor: duplicate delete_keys entry at index %d", i)
		}
		seen[key] = struct{}{}
	}
	for i, pattern := range c.Patterns {
		if pattern == "" || len(pattern) > 4096 {
			return fmt.Errorf("otelredactor: patterns[%d] must contain 1 to 4096 bytes", i)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			// regexp's error includes the expression; deliberately do not wrap it.
			return fmt.Errorf("otelredactor: patterns[%d] is not a valid Go regular expression", i)
		}
	}
	return nil
}
