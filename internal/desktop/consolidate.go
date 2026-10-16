package desktop

import (
	"slices"
	"strings"
)

// Choice is one value the user must pick during Consolidation: their own
// (Yours) or the one from the AppImage (FromAppImage). For list values, Both
// is the merged list, offered as a third option; it is empty otherwise.
type Choice struct {
	Key          string
	Yours        string
	FromAppImage string
	Both         string
}

// Consolidation is what combining a user's existing Desktop Entry with an
// Embedded Desktop Entry needs from the user.
type Consolidation struct {
	Choices []Choice
	// Carried are the extra launch arguments and environment variables found
	// in the user's Exec, kept as Overrides.
	Carried Overrides
}

// listKeys hold ";"-separated lists, which can be merged.
var listKeys = []string{"Categories", "MimeType", "Keywords"}

// pathKeys always point at the App Folder, so they are never a Choice.
var pathKeys = []string{"Exec", "TryExec", "Icon"}

// Consolidate compares a user's Desktop Entry with an Embedded Desktop Entry.
// Only keys the user's entry sets, with a value different from the
// AppImage's, become Choices, in the order the user's entry has them.
// Translated keys and AiM's own markers are ignored.
func Consolidate(user, embedded *Entry) Consolidation {
	var c Consolidation
	for _, l := range user.lines {
		if l.group != mainGroup || l.key == "" || strings.Contains(l.key, "[") ||
			strings.HasPrefix(l.key, "X-AiM-") || slices.Contains(pathKeys, l.key) {
			continue
		}
		theirs, _ := embedded.Get(l.key)
		if theirs == l.value {
			continue
		}
		choice := Choice{Key: l.key, Yours: l.value, FromAppImage: theirs}
		if slices.Contains(listKeys, l.key) {
			choice.Both = mergeLists(l.value, theirs)
		}
		c.Choices = append(c.Choices, choice)
	}
	if exec, ok := user.Get("Exec"); ok {
		c.Carried = launchSettings(exec)
	}
	return c
}

// launchSettings finds the environment variables (from a leading "env") and
// the extra arguments in an Exec value. Field codes such as %U are left out:
// the Embedded Desktop Entry's Exec brings its own.
func launchSettings(exec string) Overrides {
	var o Overrides
	words := splitExec(exec)
	if len(words) > 0 && words[0] == "env" {
		words = words[1:]
		for len(words) > 0 && strings.Contains(words[0], "=") {
			o.Env = append(o.Env, words[0])
			words = words[1:]
		}
	}
	if len(words) > 0 {
		words = words[1:] // the program itself
	}
	for _, w := range words {
		if !(len(w) == 2 && w[0] == '%') {
			o.Args = append(o.Args, w)
		}
	}
	return o
}

// splitExec splits an Exec value into words, following the Desktop Entry
// spec's double-quote and backslash rules.
func splitExec(exec string) []string {
	var words []string
	var word strings.Builder
	inWord, quoted := false, false
	for i := 0; i < len(exec); i++ {
		ch := exec[i]
		switch {
		case quoted && ch == '\\' && i+1 < len(exec):
			i++
			word.WriteByte(exec[i])
		case ch == '"':
			quoted, inWord = !quoted, true
		case !quoted && (ch == ' ' || ch == '\t'):
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteByte(ch)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

// mergeLists joins two ";"-separated lists, keeping the order of a and then
// the items only b has.
func mergeLists(a, b string) string {
	var items []string
	for _, item := range append(strings.Split(a, ";"), strings.Split(b, ";")...) {
		if item != "" && !slices.Contains(items, item) {
			items = append(items, item)
		}
	}
	return strings.Join(items, ";") + ";"
}
