package desktop

import (
	"maps"
	"slices"
	"strings"
)

const mainGroup = "Desktop Entry"

// Target is where a Managed App's files live, which the Desktop Entry must
// point at.
type Target struct {
	AppID    string
	AppImage string
	Icon     string
}

// Overrides are the user's values, applied on top of the Embedded Desktop
// Entry every time it is rebuilt.
type Overrides struct {
	// Fields maps Desktop Entry keys (Name, Comment, Categories, Keywords…)
	// to the user's value. Overriding a key also drops its translations, so
	// the user's value shows in every language.
	Fields map[string]string
	// Args are extra launch arguments, placed before the embedded ones.
	Args []string
	// Env are KEY=VALUE environment variables set for every launch.
	Env []string
}

// Build makes the Desktop Entry for a Managed App from its Embedded Desktop
// Entry: every Exec runs the installed AppImage (keeping its arguments and
// field codes), Icon and TryExec point at the App Folder, and the entry is
// marked as managed by AiM. embedded is not changed.
func Build(embedded *Entry, t Target, o Overrides) *Entry {
	e := &Entry{lines: slices.Clone(embedded.lines)}
	for i, l := range e.lines {
		if l.key == "Exec" {
			e.lines[i] = kv(l.group, "Exec", replaceProgram(l.value, launcher(t.AppImage, o)))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(o.Fields)) {
		e.deleteTranslations(mainGroup, key)
		e.set(mainGroup, key, o.Fields[key])
	}
	e.set(mainGroup, "Icon", t.Icon)
	e.set(mainGroup, "TryExec", t.AppImage)
	e.set(mainGroup, "X-AiM-Managed", "true")
	e.set(mainGroup, "X-AiM-AppId", t.AppID)
	return e
}

// launcher is the start of every Exec: optional environment, the AppImage
// and the user's extra arguments.
func launcher(appImage string, o Overrides) string {
	var words []string
	if len(o.Env) > 0 {
		words = append(words, "env")
		for _, kv := range o.Env {
			words = append(words, quote(kv))
		}
	}
	words = append(words, quote(appImage))
	for _, arg := range o.Args {
		words = append(words, quote(arg))
	}
	return strings.Join(words, " ")
}

// set gives key a value in group, replacing an existing line or adding one
// after the group's last key.
func (e *Entry) set(group, key, value string) {
	last := -1
	for i, l := range e.lines {
		if l.group != group || l.key == "" {
			continue
		}
		if l.key == key {
			e.lines[i] = kv(group, key, value)
			return
		}
		last = i
	}
	e.lines = slices.Insert(e.lines, last+1, kv(group, key, value))
}

// deleteTranslations removes the translated variants (key[xx]) of key.
func (e *Entry) deleteTranslations(group, key string) {
	e.lines = slices.DeleteFunc(e.lines, func(l line) bool {
		return l.group == group && strings.HasPrefix(l.key, key+"[")
	})
}

func kv(group, key, value string) line {
	return line{raw: key + "=" + value, group: group, key: key, value: value}
}

// replaceProgram swaps the program in an Exec value for program, keeping the
// arguments.
func replaceProgram(exec, program string) string {
	exec = strings.TrimSpace(exec)
	end := strings.IndexByte(exec, ' ')
	if strings.HasPrefix(exec, `"`) {
		if closing := strings.IndexByte(exec[1:], '"'); closing >= 0 {
			end = closing + 2
		}
	}
	if end < 0 || end >= len(exec) {
		return program
	}
	return program + exec[end:]
}

// quote quotes an Exec argument when the Desktop Entry spec requires it.
func quote(arg string) string {
	if !strings.ContainsAny(arg, " \t\"'\\><~|&;$*?#()`") {
		return arg
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(arg) + `"`
}

// StripAiM returns a copy of e without AiM's X-AiM-* markers, for a Managed
// App that is being Unmanaged. Everything else, including Overrides already
// written into the entry, is kept.
func StripAiM(e *Entry) *Entry {
	return &Entry{lines: slices.DeleteFunc(slices.Clone(e.lines), func(l line) bool {
		return strings.HasPrefix(l.key, "X-AiM-")
	})}
}
