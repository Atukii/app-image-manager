package gui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"appimage-manager/internal/core"
)

// card is a rounded surface whose body can be tapped (spec GUI-1: an app
// card's body opens the app). Buttons inside it keep their own taps.
type card struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	bg       *canvas.Rectangle
	onTapped func()
	selected bool
}

func newCard(content fyne.CanvasObject, onTapped func()) *card {
	c := &card{content: content, onTapped: onTapped}
	c.bg = canvas.NewRectangle(colours().surface)
	c.bg.CornerRadius = 12
	c.bg.StrokeWidth = 1
	c.bg.StrokeColor = colours().line
	c.ExtendBaseWidget(c)
	return c
}

func (c *card) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(c.bg, container.NewPadded(c.content)))
}

func (c *card) Tapped(*fyne.PointEvent) {
	if c.onTapped != nil {
		c.onTapped()
	}
}

func (c *card) MouseIn(*desktop.MouseEvent) {
	if c.onTapped != nil {
		c.bg.StrokeColor = primary
		c.bg.Refresh()
	}
}
func (c *card) MouseMoved(*desktop.MouseEvent) {}
func (c *card) MouseOut() {
	if !c.selected {
		c.bg.StrokeColor = colours().line
		c.bg.Refresh()
	}
}

func (c *card) setSelected(on bool) {
	c.selected = on
	c.bg.StrokeColor, c.bg.FillColor = colours().line, colours().surface
	if on {
		c.bg.StrokeColor, c.bg.FillColor = primary, colours().accentSoft
	}
	c.bg.Refresh()
}

// strip is the coloured edge of an Activity card.
func strip(c color.Color) fyne.CanvasObject {
	r := canvas.NewRectangle(c)
	r.CornerRadius = 2
	r.SetMinSize(fyne.NewSize(4, 40))
	return r
}

// chip is a status chip (spec GUI-3).
func chip(a core.AppStatus) fyne.CanvasObject {
	p := colours()
	text, fg, bg := status(a)
	switch text {
	case "Reopen":
		fg, bg = p.warn, p.warnSoft
	case "Check failed":
		fg, bg = p.bad, p.badSoft
	case "Skipped":
		fg, bg = p.info, p.infoSoft
	case "Checks off":
		fg, bg = p.muted, p.sunken
	default:
		fg, bg = p.good, p.goodSoft
	}
	label := canvas.NewText(text, fg)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.TextSize = 11
	box := canvas.NewRectangle(bg)
	box.CornerRadius = 2
	return container.NewStack(box, container.New(layout.NewCustomPaddedLayout(2, 2, 8, 8), label))
}

// status is the chip text for an app: exactly one of GUI-3's five.
func status(a core.AppStatus) (text string, _, _ color.Color) {
	switch {
	case a.Reopen:
		return "Reopen", nil, nil
	case a.LastCheck != nil && a.LastCheck.Result == "failed":
		return "Check failed", nil, nil
	case a.Skipped != "":
		return "Skipped", nil, nil
	case a.Poll == "never":
		return "Checks off", nil, nil
	}
	return "Up to date", nil, nil
}

// appIcon shows an app's icon file at size.
func appIcon(path string, size float32) fyne.CanvasObject {
	img := canvas.NewImageFromFile(path)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

// heading is a small uppercase section heading.
func heading(text string) fyne.CanvasObject {
	t := canvas.NewText(text, colours().muted)
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = 11
	return t
}

// muted is secondary text that wraps.
func muted(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// toasts shows one-line confirmations at the bottom of the window for about
// seven seconds (spec GUI-7).
type toasts struct {
	layer *fyne.Container
	timer *time.Timer
}

func newToasts() *toasts {
	return &toasts{layer: container.NewVBox()}
}

func (t *toasts) show(text string) {
	p := colours()
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	bg := canvas.NewRectangle(p.raised)
	bg.CornerRadius = 12
	bg.StrokeColor, bg.StrokeWidth = primary, 1
	box := container.NewStack(bg, container.NewPadded(label))
	t.layer.Objects = []fyne.CanvasObject{layout.NewSpacer(),
		container.New(layout.NewCustomPaddedLayout(0, theme.Padding()*6, 0, 0), container.NewCenter(container.New(fixedWidth(520), box)))}
	t.layer.Refresh()
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(7*time.Second, func() {
		fyne.Do(func() {
			t.layer.Objects = nil
			t.layer.Refresh()
		})
	})
}

// fixedWidth lays its objects out at a set width (or less, when the
// window is narrower), as tall as they need.
type fixedWidth float32

func (w fixedWidth) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objects {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(float32(w), h)
}

func (w fixedWidth) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

// tabView is a row of tabs over the selected tab's content. Unlike Fyne's
// AppTabs, the selected tab is marked with the accent ink, which stays
// readable in the light theme (GUI-8).
type tabView struct {
	Items      []*container.TabItem
	OnSelected func(*container.TabItem)
	selected   int
	bar        *fyne.Container
	body       *fyne.Container
	root       fyne.CanvasObject
}

func newTabView(items ...*container.TabItem) *tabView {
	t := &tabView{Items: items, bar: container.NewHBox(), body: container.NewStack()}
	line := canvas.NewRectangle(colours().line)
	line.SetMinSize(fyne.NewSize(0, 1))
	t.root = container.NewBorder(container.NewVBox(t.bar, line), nil, nil, nil, t.body)
	t.Refresh()
	return t
}

// SelectIndex shows tab i.
func (t *tabView) SelectIndex(i int) {
	if t.selected == i {
		return
	}
	t.selected = i
	t.Refresh()
	if t.OnSelected != nil {
		t.OnSelected(t.Items[i])
	}
}

// Refresh redraws the tab row and shows the selected tab's content.
func (t *tabView) Refresh() {
	p := colours()
	t.bar.Objects = nil
	for i, item := range t.Items {
		i := i
		label := canvas.NewText(item.Text, p.muted)
		label.TextStyle = fyne.TextStyle{Bold: true}
		under := canvas.NewRectangle(color.Transparent)
		under.SetMinSize(fyne.NewSize(0, 2))
		if i == t.selected {
			label.Color, under.FillColor = p.accentInk, p.accentInk
		}
		tab := newCard(container.NewBorder(nil, under, nil, nil, container.New(layout.NewCustomPaddedLayout(4, 4, 6, 6), label)),
			func() { t.SelectIndex(i) })
		tab.bg.StrokeWidth, tab.bg.FillColor = 0, color.Transparent
		t.bar.Add(tab)
	}
	t.bar.Refresh()
	t.body.Objects = []fyne.CanvasObject{t.Items[t.selected].Content}
	t.body.Refresh()
}
