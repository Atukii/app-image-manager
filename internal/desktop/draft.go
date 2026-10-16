package desktop

import "strings"

// Draft is the Desktop Entry AiM proposes for an AppImage that has no
// Embedded Desktop Entry. It is a suggestion for the user to review, never
// written without their confirmation.
func Draft(appID string) *Entry {
	e, _ := Parse([]byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + displayName(appID) + "\n" +
		"Exec=AppRun\n" +
		"Terminal=false\n" +
		"Categories=Utility;\n"))
	return e
}

// displayName turns an App ID like "my-tool" into "My Tool".
func displayName(appID string) string {
	words := strings.FieldsFunc(appID, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
