// Package desktop reads, builds and writes Desktop Entries.
package desktop

import "strings"

// Entry is a parsed Desktop Entry. It keeps every line, including comments,
// translated keys and action groups, so rendering an unchanged Entry gives
// back the original file.
type Entry struct {
	lines []line
}

type line struct {
	raw   string
	group string // the [group] this line belongs to
	key   string // empty for comments, blank lines and group headers
	value string
}

// Parse reads a Desktop Entry file.
func Parse(data []byte) (*Entry, error) {
	e := &Entry{}
	group := ""
	for _, raw := range strings.Split(string(data), "\n") {
		l := line{raw: raw}
		trimmed := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			group = trimmed[1 : len(trimmed)-1]
			l.group = group
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			l.group = group
		default:
			key, value, _ := strings.Cut(raw, "=")
			l.group, l.key, l.value = group, strings.TrimSpace(key), strings.TrimSpace(value)
		}
		e.lines = append(e.lines, l)
	}
	return e, nil
}

// Render writes the Entry back out as a Desktop Entry file.
func (e *Entry) Render() []byte {
	raws := make([]string, len(e.lines))
	for i, l := range e.lines {
		raws[i] = l.raw
	}
	return []byte(strings.Join(raws, "\n"))
}

// Get returns the value of key in the [Desktop Entry] group.
func (e *Entry) Get(key string) (string, bool) {
	for _, l := range e.lines {
		if l.group == mainGroup && l.key == key {
			return l.value, true
		}
	}
	return "", false
}
