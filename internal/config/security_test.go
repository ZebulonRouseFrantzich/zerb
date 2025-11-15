package config

import (
	"strings"
	"testing"
)

func TestDetectSensitiveData(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
		wantNames []string
	}{
		{
			name:      "no sensitive data",
			content:   "tools = { 'node@20.0.0', 'python@3.11' }",
			wantCount: 0,
		},
		{
			name:      "API key detected",
			content:   "api_key = 'sk_live_1234567890abcdefghij'",
			wantCount: 1,
			wantNames: []string{"API Key"},
		},
		{
			name:      "token detected",
			content:   "auth_token = 'ghp_1234567890abcdefghijklmnopqrstuv'",
			wantCount: 1,
			wantNames: []string{"Token"},
		},
		{
			name:      "password detected",
			content:   "password = 'MySecretPass123'",
			wantCount: 1,
			wantNames: []string{"Password"},
		},
		{
			name:      "GitHub token in URL",
			content:   "url = 'https://ghp_abcdefghijklmnopqrstuvwxyz1234567890@github.com/repo'",
			wantCount: 1,
			wantNames: []string{"GitHub Token"},
		},
		{
			name: "multiple sensitive items",
			content: `api_key = 'sk_1234567890123456789012'
password = 'secret123'
token = 'bearer_abcdefghijklmnopqrstuvwxyz'`,
			wantCount: 3,
			wantNames: []string{"API Key", "Password", "Token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := DetectSensitiveData(tt.content)
			
			if len(findings) != tt.wantCount {
				t.Errorf("DetectSensitiveData() found %d items, want %d", len(findings), tt.wantCount)
			}

			if tt.wantNames != nil {
				for i, finding := range findings {
					if i < len(tt.wantNames) && finding.PatternName != tt.wantNames[i] {
						t.Errorf("Finding %d: got name %q, want %q", i, finding.PatternName, tt.wantNames[i])
					}
				}
			}

			// Verify findings have required fields
			for i, finding := range findings {
				if finding.PatternName == "" {
					t.Errorf("Finding %d: missing PatternName", i)
				}
				if finding.Description == "" {
					t.Errorf("Finding %d: missing Description", i)
				}
				if finding.Line == 0 {
					t.Errorf("Finding %d: missing Line number", i)
				}
				if finding.Preview == "" {
					t.Errorf("Finding %d: missing Preview", i)
				}
			}
		})
	}
}

func TestFormatSensitiveDataWarning(t *testing.T) {
	findings := []SensitiveDataFinding{
		{
			PatternName: "API Key",
			Description: "Potential API key detected",
			Line:        5,
			Preview:     "api_key = [REDACTED]",
		},
	}

	output := FormatSensitiveDataWarning(findings)
	
	wants := []string{"WARNING", "sensitive data", "line 5", "--allow-sensitive"}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Errorf("FormatSensitiveDataWarning() missing %q in output", want)
		}
	}
}
