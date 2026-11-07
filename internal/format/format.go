// Package format canonicalizes the layout of Brick source.
//
// It is deliberately conservative: it normalizes indentation (four spaces per
// level) and strips trailing whitespace, but leaves the tokens, comments and
// blank lines exactly as written. Because comments are not part of the AST, a
// formatter that re-emitted from the tree would silently delete them; working
// at the source level keeps every character the author typed except the
// leading whitespace it is responsible for.
package format

import "strings"

const indentUnit = "    " // four spaces, matching the examples and README

// Source returns src reformatted with canonical indentation.
func Source(src string) string {
	lines := strings.Split(src, "\n")
	var b strings.Builder
	depth := 0
	for i, raw := range lines {
		content := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(content) == "" {
			// Preserve a single blank line, drop the run of whitespace.
			if i < len(lines)-1 || b.Len() > 0 {
				b.WriteByte('\n')
			}
			continue
		}
		stripped := strings.TrimLeft(content, " \t")

		// A line that starts with closing brace(s) lines up with the block it
		// closes, one level out from the current depth.
		lead := 0
		for lead < len(stripped) && stripped[lead] == '}' {
			lead++
		}
		lineDepth := depth - lead
		if lineDepth < 0 {
			lineDepth = 0
		}
		b.WriteString(strings.Repeat(indentUnit, lineDepth))
		b.WriteString(stripped)
		b.WriteByte('\n')

		delta := netBraces(stripped)
		depth += delta
		if depth < 0 {
			depth = 0
		}
	}
	out := b.String()
	// Exactly one trailing newline.
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// netBraces counts the change in brace depth of a single line, ignoring braces
// that appear inside string literals or line comments.
func netBraces(line string) int {
	delta := 0
	inString := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inString {
			if c == '\\' {
				i++ // skip the escaped character
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return delta // rest of the line is a comment
			}
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

// Formatted reports whether src is already canonically formatted.
func Formatted(src string) bool {
	return Source(src) == src
}
