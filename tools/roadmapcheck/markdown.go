package main

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type occurrence struct {
	value string
	line  int
}

type document struct {
	anchors     map[string]bool
	definitions []occurrence
	ids         []occurrence
	links       []occurrence
	issues      []occurrence
}

var (
	headingPattern   = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.+?)\s*$`)
	setextPattern    = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
	anchorPattern    = regexp.MustCompile(`(?i)<a\s+(?:[^>]*?\s)?(?:id|name)=["']([^"']+)["'][^>]*>`)
	tagPattern       = regexp.MustCompile(`<[^>]*>`)
	labelPattern     = regexp.MustCompile(`^ {0,3}\[([^]]+)\]:[ \t]*(.*)$`)
	headingIDPattern = regexp.MustCompile(`^(PC|ARCH)-[0-9]+(?:\s|$)`)
	rowIDPattern     = regexp.MustCompile("^\\s*\\|\\s*`?(VAL-[0-9]+)`?\\s*\\|")
)

func concreteIDs(text string) []string {
	var ids []string
	for _, match := range idPattern.FindAllStringIndex(text, -1) {
		word := func(b byte) bool {
			return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
		}
		if match[0] > 0 && word(text[match[0]-1]) || match[1] < len(text) && word(text[match[1]]) {
			continue
		}
		ids = append(ids, text[match[0]:match[1]])
	}
	return ids
}

func parseMarkdown(input string) *document {
	doc := &document{anchors: make(map[string]bool)}
	lines := proseLines(input)
	labels := make(map[string]string)
	definitions := make(map[int]bool)
	for i, line := range lines {
		if match := labelPattern.FindStringSubmatch(line); match != nil {
			key := normalizeLabel(match[1])
			target, _, ok := destination(match[2], false)
			if !ok {
				doc.issues = append(doc.issues, occurrence{"unsupported or malformed reference definition (use one line)", i + 1})
				continue
			}
			if _, exists := labels[key]; exists {
				doc.issues = append(doc.issues, occurrence{"duplicate link reference definition " + key, i + 1})
			} else {
				labels[key] = target
			}
			definitions[i] = true
			// Validate even unused link definitions.
			doc.links = append(doc.links, occurrence{target, i + 1})
		}
	}
	for i, line := range lines {
		lineNumber := i + 1
		heading := ""
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			heading = strings.TrimSpace(match[1])
			// Closing hashes require preceding whitespace.
			if end := strings.TrimRight(heading, "#"); len(end) < len(heading) && len(end) > 0 && (end[len(end)-1] == ' ' || end[len(end)-1] == '\t') {
				heading = strings.TrimSpace(end)
			}
		} else if i+1 < len(lines) && strings.TrimSpace(line) != "" && setextPattern.MatchString(lines[i+1]) {
			heading = strings.TrimSpace(line)
		}
		if heading != "" {
			base := headingSlug(heading, labels)
			anchor := base
			for suffix := 1; doc.anchors[anchor]; suffix++ {
				anchor = base + "-" + strconv.Itoa(suffix)
			}
			doc.anchors[anchor] = true
			if headingIDPattern.MatchString(heading) {
				doc.definitions = append(doc.definitions, occurrence{concreteIDs(heading)[0], lineNumber})
			}
		}
		if match := rowIDPattern.FindStringSubmatch(line); match != nil {
			doc.definitions = append(doc.definitions, occurrence{match[1], lineNumber})
		}
		anchorText := withoutInlineCode(line)
		for _, match := range anchorPattern.FindAllStringSubmatchIndex(anchorText, -1) {
			backslashes := 0
			for j := match[0] - 1; j >= 0 && anchorText[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				doc.anchors[html.UnescapeString(anchorText[match[2]:match[3]])] = true
			}
		}
		seen := make(map[string]bool)
		for _, id := range concreteIDs(line) {
			if !seen[id] {
				doc.ids = append(doc.ids, occurrence{id, lineNumber})
				seen[id] = true
			}
		}
		if definitions[i] {
			continue
		}
		links, issues := inlineLinks(line, labels)
		for _, link := range links {
			doc.links = append(doc.links, occurrence{link, lineNumber})
		}
		for _, issue := range issues {
			doc.issues = append(doc.issues, occurrence{issue, lineNumber})
		}
	}
	return doc
}

func withoutQuotePrefix(line string) string {
	for strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
		line = strings.TrimPrefix(strings.TrimLeft(line, " "), ">")
		line = strings.TrimPrefix(line, " ")
	}
	return line
}

// fenceText removes quote and simple list prefixes for fence recognition.
func fenceText(line string) string {
	line = withoutQuotePrefix(line)
	if match := listPrefix.FindStringIndex(line); match != nil {
		line = line[match[1]:]
	}
	return line
}

var listPrefix = regexp.MustCompile(`^ {0,3}(?:[-+*]|[0-9]+[.)])[ \t]+`)

// proseLines preserves physical line numbers while excluding comments and code.
func proseLines(input string) []string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	comment := false
	var fence byte
	fenceLength := 0
	for i, original := range lines {
		candidate := fenceText(original)
		trimmed := strings.TrimLeft(candidate, " ")
		indent := len(candidate) - len(trimmed)
		if fence != 0 {
			// Code contents cannot open HTML comments or influence later prose.
			if indent <= 3 && strings.HasPrefix(trimmed, strings.Repeat(string(fence), fenceLength)) && strings.Trim(trimmed, string(fence)+" \t") == "" {
				fence = 0
			}
			lines[i] = ""
			continue
		}
		if !comment && indent <= 3 && len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			n := 0
			for n < len(trimmed) && trimmed[n] == trimmed[0] {
				n++
			}
			if n >= 3 && (trimmed[0] != '`' || !strings.Contains(trimmed[n:], "`")) {
				fence, fenceLength = trimmed[0], n
				lines[i] = ""
				continue
			}
		}
		if !comment && (indent >= 4 || strings.HasPrefix(candidate, "\t")) {
			lines[i] = ""
			continue
		}
		var visible strings.Builder
		for j := 0; j < len(original); {
			if comment {
				end := strings.Index(original[j:], "-->")
				if end < 0 {
					break
				}
				j += end + 3
				comment = false
			} else if original[j] == '`' && codeEnd(original, j) > j {
				end := codeEnd(original, j)
				visible.WriteString(original[j:end])
				j = end
			} else if strings.HasPrefix(original[j:], "<!--") {
				comment = true
				j += 4
				visible.WriteByte(' ')
			} else {
				visible.WriteByte(original[j])
				j++
			}
		}
		lines[i] = withoutQuotePrefix(visible.String())
	}
	return lines
}

// codeEnd requires an exactly matching backtick run (longer runs are content).
func codeEnd(line string, start int) int {
	n := 1
	for start+n < len(line) && line[start+n] == '`' {
		n++
	}
	for i := start + n; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		end := i + 1
		for end < len(line) && line[end] == '`' {
			end++
		}
		if end-i == n {
			return end
		}
		i = end
	}
	return start
}

func withoutInlineCode(line string) string {
	result := []byte(line)
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == '`' {
			if end := codeEnd(line, i); end > i {
				for j := i; j < end; j++ {
					result[j] = ' '
				}
				i = end - 1
			}
		}
	}
	return string(result)
}

func normalizeLabel(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(unescapeMarkdown(label)), " "))
}

func unescapeMarkdown(text string) string {
	var output strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", rune(text[i+1])) {
			i++
		}
		output.WriteByte(text[i])
	}
	return html.UnescapeString(output.String())
}

// destination reads a single-line target and optional quoted title. For an
// inline link it also consumes the closing parenthesis. Balanced path
// parentheses and backslash escapes are supported.
func destination(text string, inline bool) (string, int, bool) {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	start := i
	target := ""
	if i < len(text) && text[i] == '<' {
		i++
		start = i
		for i < len(text) && text[i] != '>' {
			if text[i] == '\\' && i+1 < len(text) {
				i++
			}
			i++
		}
		if i == len(text) {
			return "", 0, false
		}
		target = text[start:i]
		i++
	} else {
		depth := 0
		for i < len(text) {
			b := text[i]
			if b == '\\' && i+1 < len(text) {
				i += 2
				continue
			}
			if b == '(' {
				depth++
			}
			if b == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			if b == ' ' || b == '\t' {
				break
			}
			i++
		}
		if depth != 0 {
			return "", 0, false
		}
		target = text[start:i]
	}
	spaceStart := i
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if i > spaceStart && i < len(text) && (text[i] == '\'' || text[i] == '"' || text[i] == '(') {
		close := text[i]
		if close == '(' {
			close = ')'
		}
		i++
		for i < len(text) && text[i] != close {
			if text[i] == '\\' && i+1 < len(text) {
				i++
			}
			i++
		}
		if i == len(text) {
			return "", 0, false
		}
		i++
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	}
	if inline {
		if i == len(text) || text[i] != ')' {
			return "", 0, false
		}
		return target, i + 1, true
	}
	return target, i, target != "" && i == len(text)
}

func inlineLinks(line string, labels map[string]string) (links, issues []string) {
	line = withoutInlineCode(line)
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] != '[' {
			continue
		}
		start, depth := i+1, 1
		j := start
		for ; j < len(line); j++ {
			if line[j] == '\\' {
				j++
				continue
			}
			if line[j] == '[' {
				depth++
			}
			if line[j] == ']' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if j >= len(line) {
			continue
		}
		label := line[start:j]
		if j+1 < len(line) && line[j+1] == '(' {
			target, consumed, ok := destination(line[j+2:], true)
			if !ok {
				issues = append(issues, "unsupported or malformed inline link (use one line)")
				i = j
				continue
			}
			links = append(links, target)
			i = j + 1 + consumed
			continue
		}
		if j+1 < len(line) && line[j+1] == '[' {
			end := strings.IndexByte(line[j+2:], ']')
			if end < 0 {
				continue
			}
			key := line[j+2 : j+2+end]
			if key == "" {
				key = label
			}
			if _, exists := labels[normalizeLabel(key)]; !exists {
				issues = append(issues, "undefined link reference "+normalizeLabel(key))
			}
			i = j + 2 + end
			continue // Definitions are checked at their own source line.
		}
		// Undefined shortcuts are ordinary bracketed prose, not broken links.
		i = j
	}
	return links, issues
}

var referenceHeading = regexp.MustCompile(`\[([^]]+)\]\[([^]]*)\]`)

func headingSlug(heading string, labels map[string]string) string {
	// A defined full reference renders only its first bracketed label.
	for _, match := range referenceHeading.FindAllStringSubmatch(heading, -1) {
		key := match[2]
		if key == "" {
			key = match[1]
		}
		if _, exists := labels[normalizeLabel(key)]; exists {
			heading = strings.ReplaceAll(heading, match[0], match[1])
		}
	}
	// Keep rendered label text for inline links/images, drop destination text.
	for {
		start := strings.Index(heading, "](")
		if start < 0 {
			break
		}
		_, consumed, ok := destination(heading[start+2:], true)
		if !ok {
			break
		}
		heading = heading[:start] + "]" + heading[start+2+consumed:]
	}
	heading = html.UnescapeString(tagPattern.ReplaceAllString(heading, ""))
	var slug strings.Builder
	for _, r := range strings.ToLower(unescapeMarkdown(heading)) {
		if r == ' ' || r == '\t' {
			slug.WriteByte('-')
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' {
			slug.WriteRune(r)
		}
	}
	return slug.String()
}
