package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Context menu layout in logical pixels. Window pixel size is logical * menuScale,
// where menuScale is fitted to the current monitor (multi-monitor / DPI safe).
const (
	menuPad       = 5
	menuTitleH    = 14
	menuItemH     = 20
	menuItemW     = 140
	menuColorRowH = 26
	menuSwatch    = 16
	menuSwatchGap = 4

	// Adaptive window scale bounds (device-independent pixels).
	// Prefer a readable size; only shrink when the panel would overflow.
	menuScaleMin    = 1.15
	menuScaleMax    = 1.5
	menuMaxMonFracW = 0.36 // max ~36% of monitor width
	menuMaxMonFracH = 0.55 // max ~55% of monitor height
	menuMinPixelW   = 180
	menuMinPixelH   = 220

	// Cat on-screen scale bounds for menu +/- controls.
	scaleMin  = 1.0
	scaleMax  = 8.0
	scaleStep = 0.5

	// Movement speed bounds for menu +/- controls (default is 2.0).
	speedMin  = 0.5
	speedMax  = 10.0
	speedStep = 0.5
)

// colorNames maps known hex coats to short English labels (Color menu).
var colorNames = map[string]string{
	"BC83DB": "Purple",
	"85B4DD": "Blue",
	"EDAF71": "Orange",
	"FFAAD4": "Pink",
	"B1EA9D": "Green",
	"7F7F7F": "Gray",
}

// menuKind identifies a row in the context menu.
type menuKind int

const (
	menuKindHeader menuKind = iota // non-clickable section title
	menuKindSizeDown
	menuKindSizeUp
	menuKindSpeedDown
	menuKindSpeedUp
	menuKindSound
	menuKindColorStrip // one row of color swatches
	menuKindCustomInput
	menuKindCustomApply
	menuKindAutostart
	menuKindQuit
)

// menuEntry is one row in the context menu (header or clickable item).
type menuEntry struct {
	kind  menuKind
	label string
}

func (e menuEntry) height() int {
	switch e.kind {
	case menuKindHeader:
		return menuTitleH
	case menuKindColorStrip:
		return menuColorRowH
	default:
		return menuItemH
	}
}

func (e menuEntry) clickable() bool {
	return e.kind != menuKindHeader
}

// buildMenuEntries rebuilds the flat list of rows.
func (m *neko) buildMenuEntries() {
	entries := []menuEntry{
		{kind: menuKindHeader, label: "Size"},
		{kind: menuKindSizeDown, label: "Smaller"},
		{kind: menuKindSizeUp, label: "Larger"},
		{kind: menuKindHeader, label: "Speed"},
		{kind: menuKindSpeedDown, label: "Slower"},
		{kind: menuKindSpeedUp, label: "Faster"},
		{kind: menuKindHeader, label: "Sound"},
		{kind: menuKindSound, label: "Sound"},
	}
	if _, hasBase := m.colorSheets[""]; hasBase && len(m.colorOrder) > 0 {
		entries = append(entries,
			menuEntry{kind: menuKindHeader, label: "Color"},
			menuEntry{kind: menuKindColorStrip, label: "Colors"},
			menuEntry{kind: menuKindHeader, label: "Custom"},
			menuEntry{kind: menuKindCustomInput, label: "Hex"},
			menuEntry{kind: menuKindCustomApply, label: "Apply"},
		)
	}
	entries = append(entries,
		menuEntry{kind: menuKindHeader, label: "App"},
		menuEntry{kind: menuKindAutostart, label: "Autostart"},
		menuEntry{kind: menuKindQuit, label: "Quit"},
	)
	m.menuEntries = entries
}

// openColorMenu expands the window into the context menu.
func (m *neko) openColorMenu() {
	m.buildMenuEntries()
	if len(m.menuEntries) == 0 {
		return
	}
	m.menuOpen = true
	m.menuHover = -1
	m.menuHoverColor = -1
	m.customInputFocus = false
	if m.color != "" {
		m.customHexInput = m.color
	}
	m.lastSprite = ""
	// Fit scale to the monitor that currently hosts the window.
	m.menuScale = m.fitMenuScale()
	m.applyMenuWindowSize()
	m.pinMenuWindow()
}

// closeColorMenu restores the cat-sized window at the cat's true position.
func (m *neko) closeColorMenu() {
	if !m.menuOpen {
		return
	}
	m.menuOpen = false
	m.menuHover = -1
	m.menuHoverColor = -1
	m.customInputFocus = false
	m.lastSprite = ""
	m.applyCatWindowSize()
	ebiten.SetWindowPosition(int(math.Round(m.x)), int(math.Round(m.y)))
}

func (m *neko) applyCatWindowSize() {
	ebiten.SetWindowSize(
		int(float64(width)*m.cfg.Scale),
		int(float64(height)*m.cfg.Scale),
	)
}

func (m *neko) activeMenuScale() float64 {
	if m.menuScale <= 0 {
		return 1
	}
	return m.menuScale
}

// fitMenuScale picks a window scale so the panel fits this monitor.
// Uses device-independent sizes from ebiten (works across multi-monitor / DPI).
func (m *neko) fitMenuScale() float64 {
	lw, lh := m.menuSize()
	if lw < 1 {
		lw = 1
	}
	if lh < 1 {
		lh = 1
	}

	monW, monH := m.currentMonitorSize()
	maxW := int(float64(monW) * menuMaxMonFracW)
	maxH := int(float64(monH) * menuMaxMonFracH)
	if maxW < menuMinPixelW {
		maxW = menuMinPixelW
	}
	if maxH < menuMinPixelH {
		maxH = menuMinPixelH
	}
	// Never claim more than the monitor itself.
	if monW > 0 && maxW > monW-8 {
		maxW = monW - 8
	}
	if monH > 0 && maxH > monH-8 {
		maxH = monH - 8
	}

	// Prefer the max scale when the monitor has room; only drop when needed.
	scale := menuScaleMax
	fit := math.Min(float64(maxW)/float64(lw), float64(maxH)/float64(lh))
	if fit < scale {
		scale = fit
	}
	if scale < menuScaleMin {
		// Still too tall/wide even at min: allow slightly smaller to stay on-screen.
		if fit < menuScaleMin {
			scale = fit
		} else {
			scale = menuScaleMin
		}
	}
	if scale < 1.0 {
		scale = 1.0 // never unreadable
	}
	return scale
}

func (m *neko) currentMonitorSize() (int, int) {
	if mon := ebiten.Monitor(); mon != nil {
		if w, h := mon.Size(); w > 0 && h > 0 {
			return w, h
		}
	}
	// Fallback: common laptop resolution.
	return 1280, 720
}

// applyMenuWindowSize sizes the menu window using the fitted scale.
func (m *neko) applyMenuWindowSize() {
	w, h := m.menuSize()
	s := m.activeMenuScale()
	ebiten.SetWindowSize(int(float64(w)*s), int(float64(h)*s))
}

// menuWindowPixelSize is the on-screen size of the menu panel.
func (m *neko) menuWindowPixelSize() (int, int) {
	w, h := m.menuSize()
	s := m.activeMenuScale()
	return int(float64(w) * s), int(float64(h) * s)
}

func (m *neko) menuSize() (int, int) {
	h := menuPad * 2
	for _, e := range m.menuEntries {
		h += e.height()
	}
	if h < menuPad*2+menuItemH {
		h = menuPad*2 + menuItemH
	}
	// Width grows slightly if many color swatches need room.
	w := menuPad*2 + menuItemW
	if n := len(m.colorOrder); n > 0 {
		need := menuPad*2 + n*(menuSwatch+menuSwatchGap) + menuSwatchGap
		if need > w {
			w = need
		}
	}
	return w, h
}

// clampedMenuPosition keeps the menu fully inside the current monitor.
func (m *neko) clampedMenuPosition() (int, int) {
	pw, ph := m.menuWindowPixelSize()
	// Anchor to live window position when available (multi-monitor safe).
	x, y := ebiten.WindowPosition()
	// Prefer tracked cat coords if they match the same monitor frame.
	if mx, my := int(math.Round(m.x)), int(math.Round(m.y)); mx != 0 || my != 0 {
		x, y = mx, my
	}

	monW, monH := m.currentMonitorSize()

	catW := int(float64(width) * m.cfg.Scale)
	catH := int(float64(height) * m.cfg.Scale)
	if catW < 1 {
		catW = width
	}
	if catH < 1 {
		catH = height
	}

	// If the panel would hang past the bottom/right, flip to open upward/left
	// aligned with the cat's far edge.
	if y+ph > monH {
		y = y + catH - ph
	}
	if x+pw > monW {
		x = x + catW - pw
	}

	if x+pw > monW {
		x = monW - pw
	}
	if y+ph > monH {
		y = monH - ph
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

func (m *neko) pinMenuWindow() {
	x, y := m.clampedMenuPosition()
	ebiten.SetWindowPosition(x, y)
}

// menuRowGeometry returns the Y band for entry idx.
func (m *neko) menuRowGeometry(idx int) (y0, y1 int, ok bool) {
	if idx < 0 || idx >= len(m.menuEntries) {
		return 0, 0, false
	}
	y := menuPad
	for i, e := range m.menuEntries {
		h := e.height()
		if i == idx {
			return y, y + h, true
		}
		y += h
	}
	return 0, 0, false
}

// menuItemAt returns the clickable entry index under (mx, my), or -1.
func (m *neko) menuItemAt(mx, my int) int {
	w, _ := m.menuSize()
	if mx < menuPad || mx >= w-menuPad {
		return -1
	}
	for i, e := range m.menuEntries {
		if !e.clickable() {
			continue
		}
		y0, y1, ok := m.menuRowGeometry(i)
		if !ok {
			continue
		}
		if my >= y0 && my < y1 {
			return i
		}
	}
	return -1
}

// colorStripIndex returns which coat swatch is under (mx, my) on the strip row.
func (m *neko) colorStripIndex(mx, my int) int {
	for i, e := range m.menuEntries {
		if e.kind != menuKindColorStrip {
			continue
		}
		y0, y1, ok := m.menuRowGeometry(i)
		if !ok || my < y0 || my >= y1 {
			return -1
		}
		x := menuPad + menuSwatchGap
		for ci := range m.colorOrder {
			if mx >= x && mx < x+menuSwatch {
				return ci
			}
			x += menuSwatch + menuSwatchGap
		}
		return -1
	}
	return -1
}

// setScale clamps and applies a new cat scale. The open menu stays the same size.
func (m *neko) setScale(scale float64) {
	scale = math.Round(scale/scaleStep) * scaleStep
	if scale < scaleMin {
		scale = scaleMin
	}
	if scale > scaleMax {
		scale = scaleMax
	}
	m.cfg.Scale = scale
	if !m.menuOpen {
		m.applyCatWindowSize()
	}
	m.lastSprite = ""
}

// setSpeed clamps and applies movement speed.
func (m *neko) setSpeed(speed float64) {
	speed = math.Round(speed/speedStep) * speedStep
	if speed < speedMin {
		speed = speedMin
	}
	if speed > speedMax {
		speed = speedMax
	}
	m.cfg.Speed = speed
}

// updateCustomHexInput handles keyboard when the hex field is focused.
func (m *neko) updateCustomHexInput() {
	chars := ebiten.AppendInputChars(nil)
	for _, r := range chars {
		if len(m.customHexInput) >= 6 {
			break
		}
		if !isHexRune(r) {
			continue
		}
		m.customHexInput += strings.ToUpper(string(r))
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if len(m.customHexInput) > 0 {
			m.customHexInput = m.customHexInput[:len(m.customHexInput)-1]
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		m.tryApplyCustomHex()
	}
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'f') ||
		(r >= 'A' && r <= 'F') ||
		unicode.Is(unicode.Hex_Digit, r)
}

// tryApplyCustomHex validates the buffer and applies the coat.
func (m *neko) tryApplyCustomHex() bool {
	hex := normalizeColor(m.customHexInput)
	if !hexColorRE.MatchString(hex) {
		return false
	}
	if !m.applyCustomColor(hex) {
		return false
	}
	m.buildMenuEntries()
	m.menuScale = m.fitMenuScale()
	m.applyMenuWindowSize()
	m.pinMenuWindow()
	m.customInputFocus = false
	return true
}

// updateColorMenu handles input while the context menu is open.
// Returns ebiten.Termination when the user chooses Quit.
func (m *neko) updateColorMenu() error {
	// Re-fit if the window moved to another monitor with a different size.
	if mon := ebiten.Monitor(); mon != nil && mon != m.monitor {
		m.monitor = mon
		m.menuScale = m.fitMenuScale()
		m.applyMenuWindowSize()
	}
	m.pinMenuWindow()

	mx, my := ebiten.CursorPosition()
	m.menuHover = m.menuItemAt(mx, my)
	m.menuHoverColor = m.colorStripIndex(mx, my)

	if m.customInputFocus {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			m.customInputFocus = false
			return nil
		}
		m.updateCustomHexInput()
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			m.closeColorMenu()
			return nil
		}
	} else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		m.closeColorMenu()
		return nil
	}

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}

	// Color swatch click takes priority when over the strip.
	if ci := m.colorStripIndex(mx, my); ci >= 0 && ci < len(m.colorOrder) {
		m.customInputFocus = false
		_ = m.setColor(m.colorOrder[ci])
		m.closeColorMenu()
		return nil
	}

	idx := m.menuItemAt(mx, my)
	if idx < 0 {
		m.customInputFocus = false
		m.closeColorMenu()
		return nil
	}

	entry := m.menuEntries[idx]
	switch entry.kind {
	case menuKindSizeDown:
		m.customInputFocus = false
		if m.cfg.Scale > scaleMin {
			m.setScale(m.cfg.Scale - scaleStep)
		}
	case menuKindSizeUp:
		m.customInputFocus = false
		if m.cfg.Scale < scaleMax {
			m.setScale(m.cfg.Scale + scaleStep)
		}
	case menuKindSpeedDown:
		m.customInputFocus = false
		if m.cfg.Speed > speedMin {
			m.setSpeed(m.cfg.Speed - speedStep)
		}
	case menuKindSpeedUp:
		m.customInputFocus = false
		if m.cfg.Speed < speedMax {
			m.setSpeed(m.cfg.Speed + speedStep)
		}
	case menuKindSound:
		m.customInputFocus = false
		m.cfg.Quiet = !m.cfg.Quiet
		if m.cfg.Quiet && m.currentPlayer != nil {
			_ = m.currentPlayer.Close()
			m.currentPlayer = nil
		}
	case menuKindColorStrip:
		// handled above via colorStripIndex
	case menuKindCustomInput:
		m.customInputFocus = true
	case menuKindCustomApply:
		if m.tryApplyCustomHex() {
			m.closeColorMenu()
		}
	case menuKindAutostart:
		m.customInputFocus = false
		if autostartSupported {
			_ = setAutostartEnabled(!isAutostartEnabled())
		}
	case menuKindQuit:
		return ebiten.Termination
	}
	return nil
}

// drawColorMenu paints the context menu panel.
func (m *neko) drawColorMenu(screen *ebiten.Image) {
	w, h := m.menuSize()
	screen.Clear()

	vector.FillRect(screen, 0, 0, float32(w), float32(h), color.RGBA{R: 28, G: 28, B: 32, A: 240}, false)
	vector.StrokeRect(screen, 0.5, 0.5, float32(w)-1, float32(h)-1, 1, color.RGBA{R: 90, G: 90, B: 100, A: 255}, false)

	y := menuPad
	for i, e := range m.menuEntries {
		switch e.kind {
		case menuKindHeader:
			label := e.label
			switch label {
			case "Size":
				label = fmt.Sprintf("Size x%.1f", m.cfg.Scale)
			case "Speed":
				label = fmt.Sprintf("Speed %.1f", m.cfg.Speed)
			case "Color":
				if m.menuHoverColor >= 0 && m.menuHoverColor < len(m.colorOrder) {
					label = "Color · " + colorLabel(m.colorOrder[m.menuHoverColor])
				} else {
					label = "Color · " + colorLabel(m.color)
				}
			}
			ebitenutil.DebugPrintAt(screen, label, menuPad, y-1)
			y += menuTitleH

		default:
			hovered := i == m.menuHover
			disabled := (e.kind == menuKindSizeDown && m.cfg.Scale <= scaleMin) ||
				(e.kind == menuKindSizeUp && m.cfg.Scale >= scaleMax) ||
				(e.kind == menuKindSpeedDown && m.cfg.Speed <= speedMin) ||
				(e.kind == menuKindSpeedUp && m.cfg.Speed >= speedMax) ||
				(e.kind == menuKindCustomApply && !hexColorRE.MatchString(normalizeColor(m.customHexInput))) ||
				(e.kind == menuKindAutostart && !autostartSupported)

			rowH := e.height()

			if e.kind == menuKindCustomInput && m.customInputFocus {
				vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
					color.RGBA{R: 50, G: 55, B: 70, A: 255}, false)
			} else if hovered && !disabled && e.kind != menuKindColorStrip {
				vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
					color.RGBA{R: 45, G: 45, B: 55, A: 255}, false)
			} else if disabled {
				vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
					color.RGBA{R: 32, G: 32, B: 36, A: 255}, false)
			}

			switch e.kind {
			case menuKindSizeDown, menuKindSizeUp, menuKindSpeedDown, menuKindSpeedUp:
				label := e.label
				if disabled {
					label = "  " + label
				} else if hovered {
					label = "> " + label
				} else {
					label = "  " + label
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+2, y+(rowH-10)/2)

			case menuKindSound:
				state := "On"
				if m.cfg.Quiet {
					state = "Off"
				}
				label := "  Sound: " + state
				if hovered {
					label = "> Sound: " + state
				}
				if !m.cfg.Quiet {
					vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
						color.RGBA{R: 40, G: 60, B: 50, A: 255}, false)
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+2, y+(rowH-10)/2)

			case menuKindColorStrip:
				x := menuPad + menuSwatchGap
				sy := y + (rowH-menuSwatch)/2
				for ci, c := range m.colorOrder {
					sw := colorSwatch(c)
					vector.FillRect(screen, float32(x), float32(sy), menuSwatch, menuSwatch, sw, false)
					border := color.RGBA{R: 180, G: 180, B: 190, A: 255}
					if c == m.color {
						border = color.RGBA{R: 255, G: 255, B: 255, A: 255}
						vector.StrokeRect(screen, float32(x)-1, float32(sy)-1, menuSwatch+2, menuSwatch+2, 1, border, false)
					} else if ci == m.menuHoverColor {
						border = color.RGBA{R: 220, G: 220, B: 100, A: 255}
					}
					vector.StrokeRect(screen, float32(x), float32(sy), menuSwatch, menuSwatch, 1, border, false)
					x += menuSwatch + menuSwatchGap
				}

			case menuKindCustomInput:
				sw := color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xFF}
				if hexColorRE.MatchString(normalizeColor(m.customHexInput)) {
					sw = colorSwatch(normalizeColor(m.customHexInput))
				}
				sx := float32(menuPad + 2)
				sy := float32(y + (rowH-menuSwatch)/2)
				vector.FillRect(screen, sx, sy, menuSwatch, menuSwatch, sw, false)
				vector.StrokeRect(screen, sx, sy, menuSwatch, menuSwatch, 1, color.RGBA{R: 200, G: 200, B: 200, A: 255}, false)

				display := "#" + m.customHexInput
				if m.customInputFocus && (m.count/16)%2 == 0 {
					display += "_"
				} else if !m.customInputFocus && m.customHexInput == "" {
					display = "#______"
				}
				ebitenutil.DebugPrintAt(screen, display, menuPad+2+menuSwatch+4, y+(rowH-10)/2)

			case menuKindCustomApply:
				label := "  Apply"
				if disabled {
					label = "  Apply (6 hex)"
				} else if hovered {
					label = "> Apply"
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+2, y+(rowH-10)/2)

			case menuKindAutostart:
				state := "Off"
				if autostartSupported && isAutostartEnabled() {
					state = "On"
				}
				label := "  Auto: " + state
				if !autostartSupported {
					label = "  Auto: n/a"
				} else if hovered {
					label = "> Auto: " + state
				}
				if autostartSupported && isAutostartEnabled() {
					vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
						color.RGBA{R: 40, G: 60, B: 50, A: 255}, false)
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+2, y+(rowH-10)/2)

			case menuKindQuit:
				label := "  Quit"
				if hovered {
					label = "> Quit"
					vector.FillRect(screen, float32(menuPad), float32(y), float32(w-menuPad*2), float32(rowH),
						color.RGBA{R: 70, G: 40, B: 40, A: 255}, false)
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+2, y+(rowH-10)/2)
			}

			y += rowH
		}
	}
}

// colorLabel is the human-readable menu entry for a coat key.
func colorLabel(c string) string {
	if c == "" {
		return "Original"
	}
	if name, ok := colorNames[c]; ok {
		return name
	}
	return "#" + c
}

// colorSwatch returns a solid fill for the menu chip.
func colorSwatch(c string) color.RGBA {
	if c == "" {
		return color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}
	}
	if len(c) != 6 {
		return color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
	}
	r, err1 := strconv.ParseUint(c[0:2], 16, 8)
	g, err2 := strconv.ParseUint(c[2:4], 16, 8)
	b, err3 := strconv.ParseUint(c[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xFF}
}
