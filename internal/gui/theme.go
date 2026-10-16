package gui

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The user's violet scheme (spec GUI-8). Dark is the reference; light is
// derived from the same hues.
var (
	primary   = rgb(0xa7, 0x8b, 0xfa)
	secondary = rgb(0x8b, 0x5c, 0xf6)
	tertiary  = rgb(0xc4, 0xb5, 0xfd)
	neutral   = rgb(0x0f, 0x08, 0x1d)
	salmon    = rgb(0xf4, 0xa7, 0x9a)
)

// palette is one theme variant's colours, including the status colours
// Fyne's theme has no names for.
type palette struct {
	page, surface, sunken, raised, line color.Color
	text, muted                         color.Color
	accentInk, accentSoft, onAccent     color.Color
	good, goodSoft, warn, warnSoft      color.Color
	bad, badSoft, info, infoSoft        color.Color
	cast, dim, scrim                    color.Color
}

var dark = palette{
	page: neutral, surface: rgb(0x1d, 0x18, 0x2b), sunken: rgb(0x17, 0x12, 0x23), raised: rgb(0x2a, 0x24, 0x40), line: rgb(0x33, 0x2c, 0x48),
	text: rgb(0xec, 0xe6, 0xff), muted: rgb(0xa6, 0x9f, 0xbd),
	accentInk: tertiary, accentSoft: rgb(0x2c, 0x23, 0x50), onAccent: rgb(0x16, 0x0d, 0x2e),
	good: rgb(0x8f, 0xd6, 0xa4), goodSoft: rgb(0x1d, 0x2f, 0x27), warn: rgb(0xf2, 0xc2, 0x7b), warnSoft: rgb(0x34, 0x29, 0x1b),
	bad: salmon, badSoft: rgb(0x3a, 0x20, 0x26), info: tertiary, infoSoft: rgb(0x2a, 0x23, 0x48),
	cast: color.NRGBA{0, 0, 0, 0x73}, dim: color.NRGBA{5, 2, 12, 0x47}, scrim: color.NRGBA{5, 2, 12, 0x99},
}

var light = palette{
	page: rgb(0xec, 0xe7, 0xf8), surface: color.White, sunken: rgb(0xf4, 0xf1, 0xfb), raised: rgb(0xe9, 0xe3, 0xf8), line: rgb(0xd9, 0xd1, 0xee),
	text: rgb(0x17, 0x0f, 0x2b), muted: rgb(0x5f, 0x57, 0x7a),
	accentInk: rgb(0x6a, 0x3f, 0xdc), accentSoft: rgb(0xef, 0xe9, 0xff), onAccent: rgb(0x16, 0x0d, 0x2e),
	good: rgb(0x2c, 0x7a, 0x49), goodSoft: rgb(0xe3, 0xf3, 0xe8), warn: rgb(0x94, 0x60, 0x06), warnSoft: rgb(0xfb, 0xf0, 0xdc),
	bad: rgb(0xb2, 0x46, 0x3b), badSoft: rgb(0xfb, 0xe6, 0xe2), info: rgb(0x6a, 0x3f, 0xdc), infoSoft: rgb(0xee, 0xe8, 0xff),
	cast: color.NRGBA{46, 26, 92, 0x29}, dim: color.NRGBA{23, 15, 43, 0x0d}, scrim: color.NRGBA{23, 15, 43, 0x59},
}

// variant is the theme variant in use; renders for review can force one.
var forcedVariant *fyne.ThemeVariant

// colours returns the palette for the variant in use.
func colours() palette {
	if forcedVariant != nil {
		if *forcedVariant == theme.VariantLight {
			return light
		}
		return dark
	}
	if fyne.CurrentApp() != nil && fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight {
		return light
	}
	return dark
}

func rgb(r, g, b uint8) color.Color { return color.NRGBA{r, g, b, 0xff} }

var (
	//go:embed icons/app.png
	appIconPNG []byte
	//go:embed icons/tray.png
	trayPNG []byte
	//go:embed icons/tray-attention.png
	trayAttentionPNG []byte

	// AppIcon is AiM's logo: the window and application icon.
	AppIcon       = fyne.NewStaticResource("aim.png", appIconPNG)
	trayCalm      = fyne.NewStaticResource("aim-tray.png", trayPNG)
	trayAttention = fyne.NewStaticResource("aim-tray-attention.png", trayAttentionPNG)

	//go:embed fonts/JetBrainsMono-Regular.ttf
	monoRegular []byte
	//go:embed fonts/JetBrainsMono-Bold.ttf
	monoBold []byte
	//go:embed fonts/JetBrainsMono-Italic.ttf
	monoItalic []byte
	//go:embed fonts/JetBrainsMono-BoldItalic.ttf
	monoBoldItalic []byte

	fontRegular    = fyne.NewStaticResource("JetBrainsMono-Regular.ttf", monoRegular)
	fontBold       = fyne.NewStaticResource("JetBrainsMono-Bold.ttf", monoBold)
	fontItalic     = fyne.NewStaticResource("JetBrainsMono-Italic.ttf", monoItalic)
	fontBoldItalic = fyne.NewStaticResource("JetBrainsMono-BoldItalic.ttf", monoBoldItalic)
)

// violet is aim-gui's Fyne theme.
type violet struct{}

func (violet) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if forcedVariant != nil {
		variant = *forcedVariant
	}
	p := dark
	if variant == theme.VariantLight {
		p = light
	}
	switch name {
	case theme.ColorNameBackground:
		return p.surface
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNamePrimary:
		return primary
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError:
		return p.onAccent
	case theme.ColorNameButton:
		return p.raised
	case theme.ColorNameDisabledButton:
		return p.sunken
	case theme.ColorNameInputBackground:
		return p.sunken
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return p.line
	case theme.ColorNameHover:
		return withAlpha(primary, 0x24)
	case theme.ColorNamePressed:
		return withAlpha(primary, 0x3d)
	case theme.ColorNameFocus:
		return p.accentInk
	case theme.ColorNameSelection:
		return withAlpha(primary, 0x55)
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return p.muted
	case theme.ColorNameError:
		return salmon
	case theme.ColorNameSuccess:
		return p.good
	case theme.ColorNameWarning:
		return p.warn
	case theme.ColorNameHeaderBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return p.surface
	case theme.ColorNameShadow:
		return p.cast
	case theme.ColorNameHyperlink:
		return p.accentInk
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (violet) Font(style fyne.TextStyle) fyne.Resource {
	switch {
	case style.Bold && style.Italic:
		return fontBoldItalic
	case style.Bold:
		return fontBold
	case style.Italic:
		return fontItalic
	}
	return fontRegular
}

func (violet) Icon(name fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(name) }

func (violet) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 2 // square controls
	case theme.SizeNameScrollBarRadius:
		return 2
	}
	return theme.DefaultTheme().Size(name)
}

func withAlpha(c color.Color, a uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), a}
}

// trayIcon is the logo's face, with its pink update badge when apps need
// attention (GUI-6).
func trayIcon(attention int) fyne.Resource {
	if attention > 0 {
		return trayAttention
	}
	return trayCalm
}
