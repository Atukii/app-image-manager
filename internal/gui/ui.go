package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"appimage-manager/internal/core"
)

// UI is aim-gui's window and tray icon.
type UI struct {
	app    fyne.App
	win    fyne.Window
	client *Client

	apps  []core.AppStatus
	found []core.Found

	tabs     *tabView
	activity *fyne.Container
	appsTab  *fyne.Container
	split    *splitLayout
	toast    *toasts
	openID   string // the app whose panel is open in Apps, or ""
	panelObj fyne.CanvasObject
}

// New builds aim-gui's window around client.
func New(app fyne.App, client *Client) *UI {
	app.Settings().SetTheme(violet{})
	app.SetIcon(AppIcon)
	u := &UI{app: app, client: client, toast: newToasts()}
	u.win = app.NewWindow("AiM")
	u.win.SetIcon(AppIcon)
	u.win.Resize(fyne.NewSize(1040, 680))

	toolbar := container.NewHBox(
		primaryButton("+ Add app", u.addApp),
		widget.NewButton("Find AppImages…", u.findAppImages),
		widget.NewButton("Check all", func() { u.checkAll() }),
		layout.NewSpacer(),
		widget.NewButton("Settings", u.settings),
	)
	u.activity = container.NewVBox()
	u.appsTab = container.NewVBox()
	u.split = &splitLayout{left: 330}
	appsArea := container.New(u.split, container.NewVScroll(u.appsTab))
	u.tabs = newTabView(
		container.NewTabItem("Activity", container.NewVScroll(container.NewPadded(u.activity))),
		container.NewTabItem("Apps", appsArea),
	)
	u.tabs.OnSelected = func(*container.TabItem) { u.render() }
	main := container.NewBorder(container.NewPadded(toolbar), nil, nil, nil, u.tabs.root)
	u.win.SetContent(container.NewStack(main, u.toast.layer))

	u.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		// Fyne only receives dropped files; links are pasted (GUI-4).
		for _, uri := range uris {
			if strings.HasSuffix(strings.ToLower(uri.Path()), ".appimage") {
				u.addFile(uri.Path(), "")
				return
			}
		}
		u.toast.show("Only AppImage files can be dropped here. To add from a link, paste it (Ctrl+V).")
	})
	u.win.Canvas().AddShortcut(&fyne.ShortcutPaste{}, func(fyne.Shortcut) {
		if link := strings.TrimSpace(app.Clipboard().Content()); regexp.MustCompile(`^https?://`).MatchString(link) {
			u.addLink(link)
		}
	})
	u.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == fyne.KeyEscape && u.openID != "" {
			u.closePanel()
		}
	})
	u.setUpTray()
	return u
}

// Window is aim-gui's main window.
func (u *UI) Window() fyne.Window { return u.win }

// Reload fetches the apps and found AppImages from aim and redraws.
func (u *UI) Reload() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		apps, err := u.client.List(ctx)
		found, _ := u.client.Scan(ctx)
		fyne.Do(func() {
			if err != nil {
				u.toast.show("Couldn't read your apps: " + err.Error())
				return
			}
			u.apps, u.found = apps, found
			u.render()
			u.updateTray()
		})
	}()
}

// render redraws both tabs from u.apps and u.found.
func (u *UI) render() {
	needs := u.needsYou()
	u.tabs.Items[0].Text = "Activity"
	if len(needs) > 0 {
		u.tabs.Items[0].Text = fmt.Sprintf("Activity (%d)", len(needs))
	}
	u.tabs.Items[1].Text = fmt.Sprintf("Apps (%d)", len(u.apps))
	u.tabs.Refresh()
	u.renderActivity(needs)
	u.renderApps()
}

// needsYou are the apps with a decision to make (GUI-1).
func (u *UI) needsYou() []core.AppStatus {
	var needs []core.AppStatus
	for _, a := range u.apps {
		failed := a.LastCheck != nil && a.LastCheck.Result == "failed"
		if failed || (a.Reopen && !a.Dismissed) {
			needs = append(needs, a)
		}
	}
	return needs
}

func (u *UI) renderActivity(needs []core.AppStatus) {
	p := colours()
	var objects []fyne.CanvasObject
	objects = append(objects, heading("NEEDS YOU"))
	for _, a := range needs {
		a := a
		var title, sub string
		var edge = p.warn
		var choices []fyne.CanvasObject
		if a.LastCheck != nil && a.LastCheck.Result == "failed" {
			edge = p.bad
			title = a.Name + " couldn't be checked"
			sub = firstLine(a.LastCheck.Error)
			if a.LastCheck.RateLimited {
				sub = "GitHub allows 60 lookups an hour without a token, and that limit was reached."
				choices = append(choices, primaryButton("Add token", u.settings))
			}
			choices = append(choices, widget.NewButton("Try again", func() { u.check(a.ID) }))
		} else {
			title = fmt.Sprintf("%s updated to %s", a.Name, a.Version)
			sub = fmt.Sprintf("Installed while it was open. Reopen %s to use the new version.", a.Name)
			if a.Previous != "" {
				choices = append(choices, widget.NewButton("Roll back to "+a.Previous, func() { u.rollback(a) }))
			}
			choices = append(choices, primaryButton("Got it", func() { u.simple("Dismissed.", "dismiss", a.ID) }))
		}
		body := container.NewBorder(nil, nil, container.NewHBox(strip(edge), middle(appIcon(a.Icon, 34))), middle(container.NewHBox(choices...)),
			container.NewVBox(bold(title), muted(sub)))
		objects = append(objects, newCard(body, func() { u.openApp(a.ID) }))
	}
	if len(needs) == 0 {
		objects = append(objects, newCard(container.NewCenter(muted("Nothing needs your attention.")), nil))
	}
	if len(u.found) > 0 {
		objects = append(objects, widget.NewSeparator(), heading("FOUND ON THIS COMPUTER"))
		for _, f := range u.found {
			f := f
			where := "In " + tildeHome(filepath.Dir(f.Path))
			if f.SourceURL != "" {
				where += " · " + strings.TrimPrefix(f.SourceURL, "https://")
			}
			name := filepath.Base(f.Path)
			body := container.NewBorder(nil, nil, strip(p.info),
				middle(container.NewHBox(
					primaryButton("Manage", func() { u.addFile(f.Path, f.SourceURL) }),
					widget.NewButton("Leave alone", func() { u.ignore(f) }),
					widget.NewButton("Skip", func() { u.skipFound(f) }),
				)),
				container.NewVBox(bold(name), muted(where)))
			objects = append(objects, newCard(body, nil))
		}
	}
	u.activity.Objects = objects
	u.activity.Refresh()
}

func (u *UI) renderApps() {
	var tiles []fyne.CanvasObject
	narrow := u.openID != ""
	for _, a := range u.apps {
		a := a
		var body fyne.CanvasObject
		if narrow {
			body = container.NewBorder(nil, nil, middle(appIcon(a.Icon, 30)), middle(chip(a)),
				container.NewVBox(bold(a.Name), muted(a.Version)))
		} else {
			body = container.NewVBox(container.NewHBox(appIcon(a.Icon, 34)), bold(a.Name), muted(a.Version), container.NewHBox(chip(a)))
		}
		c := newCard(body, func() { u.openApp(a.ID) })
		c.setSelected(a.ID == u.openID)
		tiles = append(tiles, c)
	}
	add := newCard(container.NewVBox(bold("+ Add an app"), muted("Drop an AppImage file here, paste a link, or click")), u.addApp)
	tiles = append(tiles, add)
	if narrow {
		u.appsTab.Objects = []fyne.CanvasObject{container.NewPadded(container.NewVBox(tiles...))}
	} else {
		u.appsTab.Objects = []fyne.CanvasObject{container.NewPadded(container.NewGridWrap(fyne.NewSize(180, 150), tiles...))}
	}
	u.appsTab.Refresh()
	if u.openID != "" {
		if a, ok := u.findApp(u.openID); ok {
			u.showPanel(a, false)
		} else {
			u.closePanel()
		}
	}
}

// openApp shows an app's panel in the Apps tab (GUI-1, GUI-2).
func (u *UI) openApp(id string) {
	opening := u.openID != id
	u.openID = id
	u.tabs.SelectIndex(1)
	u.renderApps()
	if a, ok := u.findApp(id); ok && opening {
		u.showPanel(a, true)
	}
}

func (u *UI) findApp(id string) (core.AppStatus, bool) {
	for _, a := range u.apps {
		if a.ID == id {
			return a, true
		}
	}
	return core.AppStatus{}, false
}

// splitLayout places the Apps list on the left and, when an app is open,
// its panel on the right, with a dimming layer and a cast shadow between
// them (GUI-2). offset and fade animate the panel in.
type splitLayout struct {
	left   float32
	offset float32 // the panel's slide-in offset
}

// Objects: [list] or [list, dim, shadow, panel].
func (s *splitLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	min := objects[0].MinSize()
	if len(objects) == 4 {
		min.Width = s.left + objects[3].MinSize().Width
		min.Height = max(min.Height, objects[3].MinSize().Height)
	}
	return min
}

func (s *splitLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 4 {
		objects[0].Move(fyne.NewPos(0, 0))
		objects[0].Resize(size)
		return
	}
	list, dim, shadow, panel := objects[0], objects[1], objects[2], objects[3]
	list.Move(fyne.NewPos(0, 0))
	list.Resize(fyne.NewSize(s.left, size.Height))
	dim.Move(fyne.NewPos(0, 0))
	dim.Resize(fyne.NewSize(s.left, size.Height))
	shadow.Move(fyne.NewPos(s.left-18+s.offset, 0))
	shadow.Resize(fyne.NewSize(18, size.Height))
	panel.Move(fyne.NewPos(s.left+s.offset, 0))
	panel.Resize(fyne.NewSize(size.Width-s.left, size.Height))
}

// showPanel puts a's panel in place; animate fades and slides it in.
func (u *UI) showPanel(a core.AppStatus, animate bool) {
	area := u.tabs.Items[1].Content.(*fyne.Container)
	p := colours()
	dim := canvas.NewRectangle(p.dim)
	shadow := canvas.NewHorizontalGradient(withAlpha(p.cast, 0), p.cast)
	bg := canvas.NewRectangle(p.surface)
	panel := container.NewStack(bg, u.panel(a))
	area.Objects = []fyne.CanvasObject{area.Objects[0], dim, shadow, panel}
	u.panelObj = panel
	if !animate || !fyne.CurrentApp().Settings().ShowAnimations() {
		u.split.offset = 0
		area.Refresh()
		return
	}
	dimTo := p.dim
	fyne.NewAnimation(220*time.Millisecond, func(f float32) {
		u.split.offset = 14 * (1 - f)
		r, g, b, al := dimTo.RGBA()
		dim.FillColor = withAlpha(rgb(uint8(r>>8), uint8(g>>8), uint8(b>>8)), uint8(float32(al>>8)*f))
		area.Refresh()
	}).Start()
}

// closePanel closes the open app's panel.
func (u *UI) closePanel() {
	u.openID = ""
	area := u.tabs.Items[1].Content.(*fyne.Container)
	area.Objects = area.Objects[:1]
	area.Refresh()
	u.renderApps()
}

// setUpTray shows the tray icon and its menu where the desktop has a tray
// (GUI-6).
func (u *UI) setUpTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	desk.SetSystemTrayIcon(trayIcon(0))
	u.updateTray()
	u.win.SetCloseIntercept(u.win.Hide) // the tray keeps AiM reachable
}

func (u *UI) updateTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	needs := u.needsYou()
	var items []*fyne.MenuItem
	for _, a := range needs {
		a := a
		label := fmt.Sprintf("%s updated to %s: reopen it to use it", a.Name, a.Version)
		if a.LastCheck != nil && a.LastCheck.Result == "failed" {
			label = a.Name + ": check failed"
		}
		items = append(items, fyne.NewMenuItem(label, func() { u.win.Show(); u.openApp(a.ID) }))
	}
	if len(needs) == 0 {
		none := fyne.NewMenuItem("Nothing needs your attention", nil)
		none.Disabled = true
		items = append(items, none)
	}
	items = append(items, fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Check all now", func() { u.checkAll() }),
		fyne.NewMenuItem("Open AiM", func() { u.win.Show(); u.win.RequestFocus() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit the tray icon (updates still run)", u.app.Quit),
	)
	menu := fyne.NewMenu("AiM", items...)
	desk.SetSystemTrayMenu(menu)
	desk.SetSystemTrayIcon(trayIcon(len(needs)))
}

// middle centres an object vertically at its natural height, so card
// buttons and chips don't stretch to the card's height.
func middle(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), o, layout.NewSpacer())
}

func primaryButton(text string, tapped func()) *widget.Button {
	b := widget.NewButton(text, tapped)
	b.Importance = widget.HighImportance
	return b
}

func dangerButton(text string, tapped func()) *widget.Button {
	b := widget.NewButton(text, tapped)
	b.Importance = widget.DangerImportance
	return b
}

func bold(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
