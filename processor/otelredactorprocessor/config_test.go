package otelredactorprocessor

import (
	"strings"
	"testing"
)

func policy() *Config {
	c := DefaultConfig()
	c.DeleteKeys = []string{"secret"}
	c.Patterns = []string{`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`}
	return c
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"empty policy", func(c *Config) { c.DeleteKeys = nil; c.Patterns = nil }},
		{"blank key", func(c *Config) { c.DeleteKeys = []string{" "} }},
		{"duplicate key", func(c *Config) { c.DeleteKeys = []string{"x", "x"} }},
		{"long key", func(c *Config) { c.DeleteKeys = []string{strings.Repeat("x", 4097)} }},
		{"many keys", func(c *Config) { c.DeleteKeys = make([]string, 1025) }},
		{"many patterns", func(c *Config) { c.Patterns = make([]string, 129) }},
		{"empty pattern", func(c *Config) { c.Patterns = []string{""} }},
		{"long pattern", func(c *Config) { c.Patterns = []string{strings.Repeat("x", 4097)} }},
		{"invalid pattern", func(c *Config) { c.Patterns = []string{"private-pattern["} }},
		{"blank marker", func(c *Config) { c.Replacement = " " }},
		{"large marker", func(c *Config) { c.MaxStringBytes = 1 }},
		{"zero depth", func(c *Config) { c.MaxDepth = 0 }},
		{"excessive depth", func(c *Config) { c.MaxDepth = 129 }},
		{"zero bytes", func(c *Config) { c.MaxStringBytes = 0 }},
		{"excessive bytes", func(c *Config) { c.MaxStringBytes = 16<<20 + 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := policy()
			tt.change(c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected invalid config")
			} else if strings.Contains(err.Error(), "private-pattern") {
				t.Fatal("error disclosed policy")
			}
			if _, err := newEngine(c); err == nil {
				t.Fatal("engine accepted invalid config")
			}
		})
	}
	if err := (*Config)(nil).Validate(); err == nil {
		t.Fatal("nil config accepted")
	}
	if err := policy().Validate(); err != nil {
		t.Fatal(err)
	}
	c := policy()
	c.Patterns = nil
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c = policy()
	c.DeleteKeys = nil
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
