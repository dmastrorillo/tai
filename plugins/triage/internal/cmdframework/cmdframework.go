// Package cmdframework parses, hashes, and tracks the bundled Claude
// slash-command markdowns that tai ships and that `tai install` writes
// into ~/.claude/commands/tai/.
//
// What lives here:
//
//   - Frontmatter — strict line-oriented parser for the six-key YAML
//     frontmatter shape every bundled command uses. Stdlib-only — a
//     full YAML library is intentionally avoided per the foundation
//     proposal's "stdlib first" rule.
//   - Body extractor — returns the body bytes (everything after the
//     closing `---` line), preserving any trailing newline.
//   - Hash — sha256 over the body bytes, formatted as
//     `sha256:<64-lower-hex>`. The deterministic identity for a
//     command's content.
//   - Ledger — the access contract for the cumulative per-command
//     hash history. The concrete storage and population mechanism
//     is owned by add-install-command; this package only exposes
//     `Ledger(verb)` returning the historical hashes, oldest-first.
//
// The hash and ledger machinery exists so `tai install` can classify
// a target file as missing / up-to-date / stale-but-untouched /
// user-modified before deciding whether to overwrite (see
// add-install-command/specs/install/spec.md).
package cmdframework

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Frontmatter is the structured shape of a bundled slash-command's
// frontmatter. Every bundled command MUST include exactly these six
// fields; extras are rejected at parse time.
type Frontmatter struct {
	Name        string
	Description string
	Category    string
	Tags        []string
	Version     int
	ContentHash string
}

// hashRe enforces the sha256:<64-lower-hex> shape on ContentHash values.
var hashRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// knownKeys are the six required top-level frontmatter keys. Any other
// key parsed at the top level is an error.
var knownKeys = map[string]struct{}{
	"name":         {},
	"description":  {},
	"category":     {},
	"tags":         {},
	"version":      {},
	"content_hash": {},
}

// Parse splits a slash-command markdown blob into its frontmatter and
// body components. It returns the parsed Frontmatter, the body bytes
// (preserving any trailing newline, excluding the closing `---` line),
// and any error.
//
// Errors:
//
//   - Missing leading `---` delimiter
//   - Missing closing `---` delimiter
//   - Unknown / extra top-level keys
//   - Missing required keys
//   - Malformed values (non-string where string expected; non-int version;
//     content_hash not matching sha256:<64-lower-hex>)
//   - version < 1
func Parse(src []byte) (Frontmatter, []byte, error) {
	openEnd, closeEnd, err := findDelimiters(src)
	if err != nil {
		return Frontmatter{}, nil, err
	}

	rawFront := src[openEnd:closeBefore(src, closeEnd)]
	fm, err := parseFrontmatter(rawFront)
	if err != nil {
		return Frontmatter{}, nil, err
	}
	return fm, src[closeEnd:], nil
}

// Body is a thin wrapper that returns just the body bytes.
func Body(src []byte) ([]byte, error) {
	_, body, err := Parse(src)
	return body, err
}

// HashBody returns the sha256:<hex> identity for body bytes.
//
// The body bytes are passed verbatim — no normalisation, no trim. A
// trailing newline produces a different hash from no trailing newline,
// because byte-level content is what `tai install` compares.
func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HashSource parses src and returns HashBody of the extracted body.
func HashSource(src []byte) (string, error) {
	body, err := Body(src)
	if err != nil {
		return "", err
	}
	return HashBody(body), nil
}

// closeBefore returns the start of the line that contains the closing
// `---` delimiter at closeEnd. closeEnd is the offset AFTER the `---\n`;
// we walk back one newline.
func closeBefore(src []byte, closeEnd int) int {
	// closeEnd points just after the `---\n`. Walk back: find the
	// preceding newline (the one that terminated the line BEFORE `---`).
	i := closeEnd - 1
	if i >= 0 && src[i] == '\n' {
		i--
	}
	// Now walk back to the newline before `---`.
	for i >= 0 && src[i] != '\n' {
		i--
	}
	return i + 1
}

// findDelimiters locates the open and close `---` lines and returns
// the byte offset AFTER each line's trailing newline:
//
//   - openEnd: offset after the leading `---\n`
//   - closeEnd: offset after the closing `---\n`
func findDelimiters(src []byte) (int, int, error) {
	openEnd, ok := lineEndOf(src, 0, "---")
	if !ok {
		return 0, 0, errors.New("missing leading `---` frontmatter delimiter")
	}
	// Closing delimiter starts at openEnd and we scan forward.
	closeEnd, ok := lineEndOf(src, openEnd, "---")
	if !ok {
		return 0, 0, errors.New("missing closing `---` frontmatter delimiter")
	}
	return openEnd, closeEnd, nil
}

// lineEndOf scans src starting at off for the next line whose trimmed
// content equals want. Returns the byte offset immediately AFTER that
// line's newline and true on success; (0, false) on no match.
func lineEndOf(src []byte, off int, want string) (int, bool) {
	for off < len(src) {
		nl := bytes.IndexByte(src[off:], '\n')
		var line []byte
		var next int
		if nl < 0 {
			line = src[off:]
			next = len(src)
		} else {
			line = src[off : off+nl]
			next = off + nl + 1
		}
		if strings.TrimSpace(string(line)) == want {
			return next, true
		}
		off = next
		if nl < 0 {
			break
		}
	}
	return 0, false
}

// parseFrontmatter parses the byte slice between `---` delimiters
// (newline-terminated lines) into a Frontmatter. It is intentionally
// narrow — it supports the exact shape the foundation contract names
// and refuses anything outside it.
//
// Supported per-line forms:
//
//	key: "quoted string value"
//	key: unquoted string value
//	key: 42                       (only for `version`)
//	tags: [a, b, c]               (inline array, comma-separated)
//	tags:                         (block array; each subsequent line
//	  - a                          starts with indented `- ` and a value)
//	  - b
//
// Indented continuation lines are recognised only for `tags`. Other
// keys must fit on a single line.
func parseFrontmatter(raw []byte) (Frontmatter, error) {
	var fm Frontmatter
	seen := map[string]bool{}

	lines := splitLines(raw)
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Skip blank lines.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Block-array entries should only appear immediately after a
		// `key:` with no inline value; handled inline below. A `-`
		// line outside that context is an error.
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "- ") {
			return Frontmatter{}, fmt.Errorf("unexpected block-array entry: %q", line)
		}

		key, value, hasColon := splitKeyValue(line)
		if !hasColon {
			return Frontmatter{}, fmt.Errorf("malformed frontmatter line: %q", line)
		}
		if _, ok := knownKeys[key]; !ok {
			return Frontmatter{}, fmt.Errorf("unknown frontmatter key: %q", key)
		}
		if seen[key] {
			return Frontmatter{}, fmt.Errorf("duplicate frontmatter key: %q", key)
		}
		seen[key] = true

		switch key {
		case "tags":
			tags, consumed, err := parseTags(value, lines[i+1:])
			if err != nil {
				return Frontmatter{}, err
			}
			fm.Tags = tags
			i += consumed
		case "version":
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return Frontmatter{}, fmt.Errorf("invalid frontmatter: version %q is not an integer", value)
			}
			fm.Version = n
		default:
			s, err := parseScalarString(value)
			if err != nil {
				return Frontmatter{}, fmt.Errorf("invalid frontmatter %s: %w", key, err)
			}
			switch key {
			case "name":
				fm.Name = s
			case "description":
				fm.Description = s
			case "category":
				fm.Category = s
			case "content_hash":
				fm.ContentHash = s
			}
		}
	}

	return fm, validateFrontmatter(fm, seen)
}

// splitKeyValue parses a line into (key, value, hasColon) where key is
// trimmed and lowercase-stable (frontmatter keys are case-sensitive in
// the spec; we preserve case), and value is the text after the first
// colon, with one optional leading space stripped.
func splitKeyValue(line string) (string, string, bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:colon])
	value := line[colon+1:]
	value = strings.TrimPrefix(value, " ")
	value = strings.TrimRight(value, " \t")
	return key, value, true
}

// parseScalarString accepts an unquoted, "double-quoted", or
// 'single-quoted' string value. Returns the unquoted text.
func parseScalarString(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			inner := v[1 : len(v)-1]
			if first == '"' {
				// Minimal escape handling: \", \\, \n. Enough for our
				// known fields; not a full YAML decoder.
				return unescapeDouble(inner), nil
			}
			return inner, nil
		}
	}
	return v, nil
}

// unescapeDouble handles the small set of escapes our frontmatter
// realistically uses: \\, \", \n.
func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			case '"':
				b.WriteByte('"')
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseTags accepts either an inline form `[a, b, c]` (in `value`) or a
// block form where subsequent lines begin with `- ` after `tags:` was
// given an empty value. Returns the parsed tags and the number of
// follow-up lines consumed.
func parseTags(value string, follow []string) ([]string, int, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
			return nil, 0, fmt.Errorf("invalid tags value: %q", value)
		}
		inner := strings.TrimSpace(value[1 : len(value)-1])
		if inner == "" {
			return nil, 0, errors.New("tags array must be non-empty")
		}
		parts := strings.Split(inner, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			s, err := parseScalarString(strings.TrimSpace(p))
			if err != nil {
				return nil, 0, fmt.Errorf("invalid tags entry: %w", err)
			}
			out = append(out, s)
		}
		return out, 0, nil
	}

	// Block form: each follow-up line that starts with indented `- ` is
	// an entry. Stop on the first line that doesn't.
	var tags []string
	consumed := 0
	for _, line := range follow {
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		s, err := parseScalarString(strings.TrimSpace(trimmed[2:]))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid tags entry: %w", err)
		}
		tags = append(tags, s)
		consumed++
	}
	if len(tags) == 0 {
		return nil, 0, errors.New("tags array must be non-empty")
	}
	return tags, consumed, nil
}

func splitLines(raw []byte) []string {
	s := string(raw)
	if s == "" {
		return nil
	}
	// Normalise CRLF to LF so a frontmatter authored on Windows (or
	// passed through a CRLF-friendly editor) parses identically to one
	// authored on POSIX.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// validateFrontmatter checks required-field presence and value
// invariants AFTER the line-level parser has populated fm.
func validateFrontmatter(fm Frontmatter, seen map[string]bool) error {
	var missing []string
	for k := range knownKeys {
		if !seen[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required frontmatter field(s): %s", strings.Join(missing, ", "))
	}
	if fm.Version < 1 {
		return errors.New("invalid frontmatter: version must be a positive integer")
	}
	if !hashRe.MatchString(fm.ContentHash) {
		return fmt.Errorf("invalid frontmatter: content_hash %q does not match sha256:<64-lower-hex>", fm.ContentHash)
	}
	return nil
}
