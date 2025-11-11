package config

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// Generator generates Lua configuration code from Go structs.
type Generator struct {
	indent string // Indentation string (default: two spaces)
}

// NewGenerator creates a new Lua config generator.
func NewGenerator() *Generator {
	return &Generator{
		indent: "  ", // Two spaces
	}
}

// Generate generates Lua code from a Config struct.
// The output is formatted and human-readable.
func (g *Generator) Generate(config *Config) (string, error) {
	var buf bytes.Buffer

	// Write header comment
	buf.WriteString("-- ZERB Configuration\n")
	buf.WriteString("-- Generated: ")
	buf.WriteString(time.Now().Format(time.RFC3339))
	buf.WriteString("\n\n")

	// Write zerb table
	buf.WriteString("zerb = {\n")

	// Write meta section
	if config.Meta.Name != "" || config.Meta.Description != "" {
		g.writeMeta(&buf, config.Meta)
	}

	// Write tools section
	if len(config.Tools) > 0 {
		g.writeTools(&buf, config.Tools)
	}

	// Write configs section
	if len(config.Configs) > 0 {
		g.writeConfigFiles(&buf, config.Configs)
	}

	// Write git section
	if config.Git.Remote != "" || config.Git.Branch != "" {
		g.writeGitConfig(&buf, config.Git)
	}

	// Write options section
	if config.Options.BackupRetention > 0 {
		g.writeOptions(&buf, config.Options)
	}

	buf.WriteString("}\n")

	return buf.String(), nil
}

// GenerateTimestamped generates a timestamped config with metadata.
func (g *Generator) GenerateTimestamped(config *Config, gitCommit string) (filename, content string, err error) {
	var buf bytes.Buffer

	// Generate timestamp
	timestamp := time.Now().UTC()
	timestampStr := timestamp.Format("20060102T150405Z")
	filename = fmt.Sprintf("zerb.lua.%s", timestampStr)

	// Write header with metadata
	buf.WriteString("-- ZERB CONFIG - Timestamped Snapshot\n")
	buf.WriteString(fmt.Sprintf("-- Created: %s\n", timestamp.Format(time.RFC3339)))
	buf.WriteString("--\n")
	buf.WriteString("-- This is a versioned snapshot. To make changes:\n")
	buf.WriteString("--   1. Edit: vim ~/.config/zerb/zerb.lua.active\n")
	buf.WriteString("--   2. Apply: zerb sync\n")
	buf.WriteString("\n")

	// Write metadata table
	buf.WriteString("-- METADATA (do not remove)\n")
	buf.WriteString("local _metadata = {\n")
	buf.WriteString(fmt.Sprintf("%sversion = 1,\n", g.indent))
	buf.WriteString(fmt.Sprintf("%stimestamp = %q,\n", g.indent, timestamp.Format(time.RFC3339)))
	if gitCommit != "" {
		buf.WriteString(fmt.Sprintf("%sgit_commit = %q,\n", g.indent, gitCommit))
	}
	buf.WriteString("}\n\n")

	// Generate main config
	configCode, err := g.Generate(config)
	if err != nil {
		return "", "", err
	}

	buf.WriteString("-- ACTUAL CONFIG\n")
	buf.WriteString(configCode)
	buf.WriteString("\nreturn zerb\n")

	return filename, buf.String(), nil
}

// writeMeta writes the meta section to the buffer.
func (g *Generator) writeMeta(buf *bytes.Buffer, meta Meta) {
	buf.WriteString(g.indent)
	buf.WriteString("meta = {\n")

	if meta.Name != "" {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("name = ")
		buf.WriteString(g.quoteLuaString(meta.Name))
		buf.WriteString(",\n")
	}

	if meta.Description != "" {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("description = ")
		buf.WriteString(g.quoteLuaString(meta.Description))
		buf.WriteString(",\n")
	}

	buf.WriteString(g.indent)
	buf.WriteString("},\n\n")
}

// writeTools writes the tools section to the buffer.
func (g *Generator) writeTools(buf *bytes.Buffer, tools []string) {
	buf.WriteString(g.indent)
	buf.WriteString("tools = {\n")

	for _, tool := range tools {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString(g.quoteLuaString(tool))
		buf.WriteString(",\n")
	}

	buf.WriteString(g.indent)
	buf.WriteString("},\n\n")
}

// writeConfigFiles writes the configs section to the buffer.
func (g *Generator) writeConfigFiles(buf *bytes.Buffer, configs []ConfigFile) {
	buf.WriteString(g.indent)
	buf.WriteString("configs = {\n")

	for _, cf := range configs {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)

		// If it's just a path with no options, write as a string
		if !cf.Recursive && !cf.Template && !cf.Secrets && !cf.Private {
			buf.WriteString(g.quoteLuaString(cf.Path))
			buf.WriteString(",\n")
			continue
		}

		// Otherwise, write as a table with options
		buf.WriteString("{\n")

		// Path
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("path = ")
		buf.WriteString(g.quoteLuaString(cf.Path))
		buf.WriteString(",\n")

		// Options
		if cf.Recursive {
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString("recursive = true,\n")
		}
		if cf.Template {
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString("template = true,\n")
		}
		if cf.Secrets {
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString("secrets = true,\n")
		}
		if cf.Private {
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString(g.indent)
			buf.WriteString("private = true,\n")
		}

		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("},\n")
	}

	buf.WriteString(g.indent)
	buf.WriteString("},\n\n")
}

// writeGitConfig writes the git section to the buffer.
func (g *Generator) writeGitConfig(buf *bytes.Buffer, git GitConfig) {
	buf.WriteString(g.indent)
	buf.WriteString("git = {\n")

	if git.Remote != "" {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("remote = ")
		buf.WriteString(g.quoteLuaString(git.Remote))
		buf.WriteString(",\n")
	}

	if git.Branch != "" {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString("branch = ")
		buf.WriteString(g.quoteLuaString(git.Branch))
		buf.WriteString(",\n")
	}

	buf.WriteString(g.indent)
	buf.WriteString("},\n\n")
}

// writeOptions writes the config/options section to the buffer.
func (g *Generator) writeOptions(buf *bytes.Buffer, options Options) {
	buf.WriteString(g.indent)
	buf.WriteString("config = {\n")

	if options.BackupRetention > 0 {
		buf.WriteString(g.indent)
		buf.WriteString(g.indent)
		buf.WriteString(fmt.Sprintf("backup_retention = %d,\n", options.BackupRetention))
	}

	buf.WriteString(g.indent)
	buf.WriteString("},\n")
}

// quoteLuaString quotes a string for Lua, handling special characters.
func (g *Generator) quoteLuaString(s string) string {
	// Use double quotes and escape special characters
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslashes first
	s = strings.ReplaceAll(s, "\"", "\\\"") // Escape double quotes
	s = strings.ReplaceAll(s, "\n", "\\n")  // Escape newlines
	s = strings.ReplaceAll(s, "\r", "\\r")  // Escape carriage returns
	s = strings.ReplaceAll(s, "\t", "\\t")  // Escape tabs
	return "\"" + s + "\""
}
