package config

import (
	"fmt"
	"regexp"
	"strings"
)

// SensitivePattern represents a pattern that might indicate sensitive data
type SensitivePattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Description string
}

var sensitivePatterns = []SensitivePattern{
	{
		Name:        "API Key",
		Pattern:     regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*=\s*['"][a-zA-Z0-9_-]{15,}['"]`),
		Description: "Potential API key detected",
	},
	{
		Name:        "Token",
		Pattern:     regexp.MustCompile(`(?i)(token|auth[_-]?token|access[_-]?token|bearer)\s*=\s*['"][a-zA-Z0-9_-]{15,}['"]`),
		Description: "Potential authentication token detected",
	},
	{
		Name:        "Password",
		Pattern:     regexp.MustCompile(`(?i)(password|passwd|pwd)\s*=\s*['"].+['"]`),
		Description: "Potential password detected",
	},
	{
		Name:        "Secret",
		Pattern:     regexp.MustCompile(`(?i)(secret|secret[_-]?key|private[_-]?key)\s*=\s*['"][a-zA-Z0-9_-]{15,}['"]`),
		Description: "Potential secret key detected",
	},
	{
		Name:        "AWS Key",
		Pattern:     regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)\s*=\s*['"][A-Z0-9]{16,}['"]`),
		Description: "Potential AWS credentials detected",
	},
	{
		Name:        "GitHub Token",
		Pattern:     regexp.MustCompile(`gh[ps]_[a-zA-Z0-9]{36,}`),
		Description: "Potential GitHub token detected",
	},
}

// SensitiveDataFinding represents a detected sensitive data instance
type SensitiveDataFinding struct {
	PatternName string
	Description string
	Line        int
	Preview     string // Redacted preview of the match
}

// DetectSensitiveData scans configuration content for potential sensitive data
func DetectSensitiveData(content string) []SensitiveDataFinding {
	var findings []SensitiveDataFinding
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, pattern := range sensitivePatterns {
			if pattern.Pattern.MatchString(line) {
				// Create redacted preview
				preview := redactSensitiveValue(line)

				findings = append(findings, SensitiveDataFinding{
					PatternName: pattern.Name,
					Description: pattern.Description,
					Line:        lineNum + 1, // 1-based line numbers
					Preview:     preview,
				})
			}
		}
	}

	return findings
}

// redactSensitiveValue creates a redacted preview of a line with sensitive data
func redactSensitiveValue(line string) string {
	// Find the assignment operator
	eqIdx := strings.Index(line, "=")
	if eqIdx == -1 {
		// If no '=', just show first 30 chars
		if len(line) > 30 {
			return line[:30] + "... [REDACTED]"
		}
		return line + " [REDACTED]"
	}

	// Show the key part, redact the value
	keyPart := strings.TrimSpace(line[:eqIdx])
	return keyPart + " = [REDACTED]"
}

// FormatSensitiveDataWarning formats findings into a user-friendly warning message
func FormatSensitiveDataWarning(findings []SensitiveDataFinding) string {
	if len(findings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n⚠️  WARNING: Potential sensitive data detected in configuration\n\n")
	sb.WriteString("The following patterns were found that may indicate hardcoded secrets:\n\n")

	for i, finding := range findings {
		sb.WriteString(fmt.Sprintf("%d. %s (line %d)\n", i+1, finding.Description, finding.Line))
		sb.WriteString(fmt.Sprintf("   Preview: %s\n\n", finding.Preview))
	}

	sb.WriteString("SECURITY RECOMMENDATION:\n")
	sb.WriteString("• Use environment variables instead of hardcoding secrets\n")
	sb.WriteString("• Example: password = os.getenv('MY_PASSWORD')\n")
	sb.WriteString("• Never commit secrets to version control\n")
	sb.WriteString("\nTo proceed anyway, use the --allow-sensitive flag.\n")

	return sb.String()
}
