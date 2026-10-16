package gui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"appimage-manager/internal/core"
)

// panel is an app's panel: everything about it on one screen, in two
// columns (GUI-2).
func (u *UI) panel(a core.AppStatus) fyne.CanvasObject {
	p := colours()
	header := container.NewBorder(nil, nil,
		appIcon(a.Icon, 46),
		middle(container.NewHBox(chip(a), widget.NewButton("✕", u.closePanel))),
		container.NewVBox(bold(a.Name), muted(fmt.Sprintf("%s · App ID %s", a.Version, a.ID))))

	var note fyne.CanvasObject = layout.NewSpacer()
	switch text, _, _ := status(a); text {
	case "Reopen":
		note = callout(fmt.Sprintf("%s is installed. %s is still running the previous version; reopen it to use the new one. AiM never closes your apps.", a.Version, a.Name), p.warn)
	case "Check failed":
		msg := "The last check failed: " + firstLine(a.LastCheck.Error)
		if a.LastCheck.RateLimited {
			msg = "The last check failed: GitHub allows 60 lookups an hour without a token. A token in Settings raises the limit."
		}
		note = callout(msg, p.bad)
	case "Skipped":
		note = callout(fmt.Sprintf("Checks skip %s until a newer release appears.", a.Skipped), p.info)
	}

	// Left column: actions, update checks, details.
	var actions []fyne.CanvasObject
	if a.Previous != "" {
		actions = append(actions, widget.NewButton("Roll back to "+a.Previous, func() { u.rollback(a) }))
	}
	if a.Skipped != "" {
		actions = append(actions, widget.NewButton("Allow "+a.Skipped, func() { u.simple("Allowed "+a.Skipped+".", "unskip", a.ID) }))
	} else {
		actions = append(actions, widget.NewButton("Skip a release…", func() { u.skipRelease(a) }))
	}
	polls := container.NewHBox()
	for _, rate := range []string{"daily", "weekly", "monthly", "never"} {
		rate := rate
		b := widget.NewButton(strings.ToUpper(rate[:1])+rate[1:], func() {
			u.simple("Checks are now "+rate+".", "set-poll", a.ID, rate)
		})
		if string(a.Poll) == rate {
			b.Importance = widget.HighImportance
		}
		polls.Add(b)
	}
	last := "Last check: never"
	if a.LastCheck != nil {
		last = "Last check: " + a.LastCheck.At.Local().Format("Mon 2 Jan 15:04") + " · " + a.LastCheck.Result
	}
	details := widget.NewForm(
		widget.NewFormItem("Source", selectable(a.Source)),
		widget.NewFormItem("File pattern", selectable(a.AssetPattern)),
		widget.NewFormItem("Files", selectable(tildeHome(dirOf(a.Icon))+"/")),
		widget.NewFormItem("Menu entry", selectable(a.DesktopFile)),
	)
	left := container.NewVBox(
		container.NewHBox(primaryButton("Check for updates", func() { u.check(a.ID) })),
		container.NewHBox(actions...),
		heading("UPDATE CHECKS"), polls, muted(last),
		heading("DETAILS"), details,
	)

	// Right column: application-menu fields, icon, launch, let go.
	name := widget.NewEntry()
	name.SetText(fieldValue(a, "Name", a.Name))
	categories := widget.NewEntry()
	categories.SetText(fieldValue(a, "Categories", ""))
	categories.SetPlaceHolder("from the AppImage")
	args := widget.NewEntry()
	args.SetText(strings.Join(a.Overrides.Args, " "))
	args.SetPlaceHolder("--ozone-platform=wayland")
	env := widget.NewEntry()
	env.SetText(strings.Join(a.Overrides.Env, " "))
	env.SetPlaceHolder("KEY=value")
	save := primaryButton("Save changes", nil)
	save.Disable()
	for _, e := range []*widget.Entry{name, categories, args, env} {
		e.OnChanged = func(string) { save.Enable() }
	}
	save.OnTapped = func() {
		edit := []string{"edit", a.ID}
		if name.Text != a.Name || a.Overrides.Fields["Name"] != "" {
			edit = append(edit, "--set", "Name="+name.Text)
		}
		if categories.Text != "" {
			edit = append(edit, "--set", "Categories="+categories.Text)
		}
		edit = append(edit, listFlags("--arg", "--clear-args", args.Text)...)
		edit = append(edit, listFlags("--env", "--clear-env", env.Text)...)
		u.simple("Saved. The menu entry is updated.", edit...)
	}
	source := func(key string) fyne.CanvasObject {
		if _, mine := a.Overrides.Fields[key]; !mine {
			return muted("From the AppImage")
		}
		return container.NewHBox(muted("Yours"), widget.NewButton("Use the AppImage's", func() {
			u.simple("Back to the AppImage's "+key+".", "edit", a.ID, "--reset", key)
		}))
	}
	icon := container.NewHBox(appIcon(a.Icon, 24), widget.NewButton("Choose…", func() { u.chooseIcon(a) }))
	menuFields := widget.NewForm(
		widget.NewFormItem("Name", container.NewVBox(name, source("Name"))),
		widget.NewFormItem("Categories", container.NewVBox(categories, source("Categories"))),
		widget.NewFormItem("Icon", icon),
	)
	launch := widget.NewForm(
		widget.NewFormItem("Arguments", args),
		widget.NewFormItem("Environment", env),
	)
	right := container.NewVBox(
		heading("IN THE APPLICATION MENU"), menuFields,
		heading("LAUNCH"), launch, container.NewHBox(layout.NewSpacer(), save),
		heading("LET GO"), container.NewHBox(
			widget.NewButton("Stop managing…", func() { u.runFlow("Stop managing "+a.Name, "Stop managing", []string{"unmanage", a.ID}) }),
			dangerButton("Remove app…", func() { u.runFlow("Remove "+a.Name, "Remove", []string{"remove", a.ID}) }),
		),
	)
	return container.NewPadded(container.NewBorder(
		container.NewVBox(header, note), nil, nil, nil,
		container.NewGridWithColumns(2, left, right)))
}

// fieldValue is the app's Desktop Entry value for key: the user's
// Override, or fallback.
func fieldValue(a core.AppStatus, key, fallback string) string {
	if v, ok := a.Overrides.Fields[key]; ok {
		return v
	}
	return fallback
}

// listFlags turns a space-separated list into repeated flags, or the
// clearing flag when it is empty.
func listFlags(flag, clear, text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{clear}
	}
	var out []string
	for _, w := range words {
		out = append(out, flag, w)
	}
	return out
}

func callout(text string, edge color.Color) fyne.CanvasObject {
	return container.NewBorder(nil, nil, strip(edge), nil, muted(text))
}

func selectable(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapBreak
	l.Selectable = true
	return l
}

func (u *UI) chooseIcon(a core.AppStatus) {
	open := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		r.Close()
		u.simple("Icon changed. AiM keeps it across updates.", "edit", a.ID, "--icon", r.URI().Path())
	}, u.win)
	open.SetFilter(storage.NewExtensionFileFilter([]string{".png", ".svg"}))
	open.Show()
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return path
}

// tildeHome shortens paths in the home directory to ~/….
func tildeHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// trayIcon draws the tray icon: a Primary download arrow, with a badge
// when apps need attention (GUI-6).
func trayIcon(attention int) fyne.Resource {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := primary.(color.NRGBA)
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(29, 8, 35, 40, ink)  // shaft
	for i := 0; i < 14; i++ { // arrow head
		fill(18+i, 26+i, 46-i, 28+i, ink)
	}
	fill(12, 50, 52, 56, ink) // tray
	if attention > 0 {
		badge := salmon.(color.NRGBA)
		for y := 0; y < 22; y++ {
			for x := 0; x < 22; x++ {
				if (x-11)*(x-11)+(y-11)*(y-11) <= 110 {
					img.SetNRGBA(size-22+x, y, badge)
				}
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return fyne.NewStaticResource(fmt.Sprintf("aim-tray-%d.png", min(attention, 1)), buf.Bytes())
}
