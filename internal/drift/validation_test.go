package drift

import "testing"

func TestValidateToolName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "node", false},
		{"valid with hyphen", "go-task", false},
		{"valid with underscore", "my_tool", false},
		{"valid with numbers", "node20", false},
		{"valid repo path", "sharkdp/bat", false},
		{"valid complex", "golang-ci/golangci-lint", false},
		{"empty string", "", true},
		{"shell metachar semicolon", "tool;rm -rf /", true},
		{"shell metachar pipe", "tool|cat", true},
		{"shell metachar ampersand", "tool&", true},
		{"shell metachar backtick", "tool`whoami`", true},
		{"shell metachar dollar", "tool$(id)", true},
		{"spaces", "my tool", true},
		{"special chars", "tool@#$%", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToolName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToolName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "1.2.3", false},
		{"valid with v prefix", "v1.2.3", false},
		{"valid pre-release", "1.2.3-beta.1", false},
		{"valid build metadata", "1.2.3+build.456", false},
		{"valid complex", "1.2.3-rc.1+build.123", false},
		{"valid with underscore", "20_11_0", false},
		{"empty string", "", true},
		{"shell metachar semicolon", "1.2.3;rm -rf /", true},
		{"shell metachar pipe", "1.2.3|cat", true},
		{"shell metachar ampersand", "1.2.3&", true},
		{"shell metachar backtick", "1.2.3`whoami`", true},
		{"shell metachar dollar", "1.2.3$(id)", true},
		{"spaces", "1.2.3 malicious", true},
		{"special chars", "1.2.3@#$%", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
