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

// Context menu layout (logical pixels). The menu always uses menuUIScale for
// the window size so changing the cat's Scale never resizes the panel.
const (
	menuPad     = 4
	menuTitleH  = 14
	menuItemH   = 22
	menuItemW   = 128
	menuPreview = 18
	menuSwatch  = 10
	menuTextX   = menuPad + menuPreview + 6

	// Fixed on-screen scale for the menu window (independent of cat Scale).
	menuUIScale = 1.5

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
	menuKindColor
	menuKindCustomInput // hex text field
	menuKindCustomApply // apply typed hex
	menuKindAutostart   // toggle run at Windows login
	menuKindQuit        // exit the program
)

// menuEntry is one row in the context menu (header or clickable item).
type menuEntry struct {
	kind  menuKind
	color string // only for menuKindColor
	label string
}

func (e menuEntry) height() int {
	if e.kind == menuKindHeader {
		return menuTitleH
	}
	return menuItemH
}

func (e menuEntry) clickable() bool {
	return e.kind != menuKindHeader
}

// buildMenuEntries rebuilds the flat list of rows.
func (m *neko) buildMenuEntries() {
	entries := []menuEntry{
		{kind: menuKindHeader, label: "Size"},
		{kind: menuKindSizeDown, label: "Size smaller"},
		{kind: menuKindSizeUp, label: "Size larger"},
		{kind: menuKindHeader, label: "Speed"},
		{kind: menuKindSpeedDown, label: "Speed slower"},
		{kind: menuKindSpeedUp, label: "Speed faster"},
		{kind: menuKindHeader, label: "Sound"},
		{kind: menuKindSound, label: "Sound"},
	}
	// Color presets + custom hex when the original base sheet is available.
	if _, hasBase := m.colorSheets[""]; hasBase && len(m.colorOrder) > 0 {
		entries = append(entries, menuEntry{kind: menuKindHeader, label: "Color"})
		for _, c := range m.colorOrder {
			entries = append(entries, menuEntry{
				kind:  menuKindColor,
				color: c,
				label: colorLabel(c),
			})
		}
		entries = append(entries,
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
	m.customInputFocus = false
	// Prefill with current coat when it is a hex color.
	if m.color != "" {
		m.customHexInput = m.color
	}
	m.lastSprite = ""
	m.applyMenuWindowSize()
	// Keep the panel on-screen (e.g. when the cat sits near the bottom edge).
	m.pinMenuWindow()
}

// closeColorMenu restores the cat-sized window at the cat's true position.
func (m *neko) closeColorMenu() {
	if !m.menuOpen {
		return
	}
	m.menuOpen = false
	m.menuHover = -1
	m.customInputFocus = false
	m.lastSprite = ""
	m.applyCatWindowSize()
	// Menu may have been shifted to fit the screen; snap back to the cat.
	ebiten.SetWindowPosition(int(math.Round(m.x)), int(math.Round(m.y)))
}

func (m *neko) applyCatWindowSize() {
	ebiten.SetWindowSize(
		int(float64(width)*m.cfg.Scale),
		int(float64(height)*m.cfg.Scale),
	)
}

// applyMenuWindowSize sizes the window for the menu using a fixed UI scale
// so the panel does not grow/shrink with the cat.
func (m *neko) applyMenuWindowSize() {
	w, h := m.menuSize()
	ebiten.SetWindowSize(
		int(float64(w)*menuUIScale),
		int(float64(h)*menuUIScale),
	)
}

// menuWindowPixelSize is the on-screen size of the menu panel.
func (m *neko) menuWindowPixelSize() (int, int) {
	w, h := m.menuSize()
	return int(float64(w) * menuUIScale), int(float64(h) * menuUIScale)
}

// clampedMenuPosition returns a top-left window position for the menu that
// stays fully inside the current monitor, preferring the cat's location.
func (m *neko) clampedMenuPosition() (int, int) {
	pw, ph := m.menuWindowPixelSize()
	x := int(math.Round(m.x))
	y := int(math.Round(m.y))

	monW, monH := 0, 0
	if mon := ebiten.Monitor(); mon != nil {
		monW, monH = mon.Size()
	}
	if monW <= 0 || monH <= 0 {
		return x, y
	}

	// Prefer opening upward when the cat is in the lower half of the screen,
	// so a tall menu does not spill off the bottom.
	if y+ph > monH {
		// Align menu bottom with cat bottom (approx: cat size scaled).
		catH := int(float64(height) * m.cfg.Scale)
		y = y + catH - ph
	}
	if x+pw > monW {
		catW := int(float64(width) * m.cfg.Scale)
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

// pinMenuWindow places the menu window at a clamped on-screen position.
func (m *neko) pinMenuWindow() {
	x, y := m.clampedMenuPosition()
	ebiten.SetWindowPosition(x, y)
}

func (m *neko) menuSize() (int, int) {
	h := menuPad * 2
	for _, e := range m.menuEntries {
		h += e.height()
	}
	if h < menuPad*2+menuItemH {
		h = menuPad*2 + menuItemH
	}
	return menuPad*2 + menuItemW, h
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
	if mx < menuPad || mx >= menuPad+menuItemW {
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
	// Typed characters (hex only, max 6).
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
	// Ctrl/Cmd+V is not handled; paste support varies by platform.

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
	// Refresh color list rows so the new coat appears as a preset entry.
	m.buildMenuEntries()
	m.applyMenuWindowSize()
	m.customInputFocus = false
	return true
}

// updateColorMenu handles input while the context menu is open.
// Returns ebiten.Termination when the user chooses Quit.
func (m *neko) updateColorMenu() error {
	// Stay on-screen; do not follow m.x/m.y raw (would clip at edges).
	m.pinMenuWindow()

	// Hex field keyboard capture takes priority while focused.
	if m.customInputFocus {
		// Esc unfocuses first; second Esc (next frame) closes menu.
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			m.customInputFocus = false
			return nil
		}
		m.updateCustomHexInput()
		// Still allow right-click to close.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			m.closeColorMenu()
			return nil
		}
		// Clicks still work for other rows / apply.
	} else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		m.closeColorMenu()
		return nil
	}

	mx, my := ebiten.CursorPosition()
	m.menuHover = m.menuItemAt(mx, my)

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
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
	case menuKindColor:
		m.customInputFocus = false
		_ = m.setColor(entry.color)
		m.closeColorMenu()
	case menuKindCustomInput:
		m.customInputFocus = true
	case menuKindCustomApply:
		if m.tryApplyCustomHex() {
			// Keep menu open so the user sees the new color in the list;
			// close so the cat shows immediately.
			m.closeColorMenu()
		}
	case menuKindAutostart:
		m.customInputFocus = false
		if !autostartSupported {
			break
		}
		// Toggle current-user login autostart (Windows Run key).
		_ = setAutostartEnabled(!isAutostartEnabled())
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
				label = fmt.Sprintf("Size  (x%.1f)", m.cfg.Scale)
			case "Speed":
				label = fmt.Sprintf("Speed (%.1f)", m.cfg.Speed)
			}
			ebitenutil.DebugPrintAt(screen, label, menuPad, y-1)
			y += menuTitleH

		default:
			selected := e.kind == menuKindColor && e.color == m.color
			hovered := i == m.menuHover
			disabled := (e.kind == menuKindSizeDown && m.cfg.Scale <= scaleMin) ||
				(e.kind == menuKindSizeUp && m.cfg.Scale >= scaleMax) ||
				(e.kind == menuKindSpeedDown && m.cfg.Speed <= speedMin) ||
				(e.kind == menuKindSpeedUp && m.cfg.Speed >= speedMax) ||
				(e.kind == menuKindCustomApply && !hexColorRE.MatchString(normalizeColor(m.customHexInput))) ||
				(e.kind == menuKindAutostart && !autostartSupported)

			switch {
			case e.kind == menuKindCustomInput && m.customInputFocus:
				vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
					color.RGBA{R: 50, G: 55, B: 70, A: 255}, false)
			case selected:
				vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
					color.RGBA{R: 55, G: 70, B: 95, A: 255}, false)
			case hovered && !disabled:
				vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
					color.RGBA{R: 45, G: 45, B: 55, A: 255}, false)
			case disabled:
				vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
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
				ebitenutil.DebugPrintAt(screen, label, menuPad+4, y+(menuItemH-12)/2)

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
					vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
						color.RGBA{R: 40, G: 60, B: 50, A: 255}, false)
					if hovered {
						vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
							color.RGBA{R: 50, G: 75, B: 60, A: 255}, false)
					}
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+4, y+(menuItemH-12)/2)

			case menuKindColor:
				previewY := y + (menuItemH-menuPreview)/2
				if sheets, ok := m.colorSheets[e.color]; ok {
					if awake, ok := sheets["awake"]; ok && awake != nil {
						op := &ebiten.DrawImageOptions{}
						scale := float64(menuPreview) / float64(width)
						op.GeoM.Scale(scale, scale)
						op.GeoM.Translate(float64(menuPad+2), float64(previewY))
						screen.DrawImage(awake, op)
					}
				}

				sw := colorSwatch(e.color)
				sx := float32(menuTextX)
				sy := float32(y + (menuItemH-menuSwatch)/2)
				vector.FillRect(screen, sx, sy, menuSwatch, menuSwatch, sw, false)
				vector.StrokeRect(screen, sx, sy, menuSwatch, menuSwatch, 1, color.RGBA{R: 200, G: 200, B: 200, A: 255}, false)

				label := e.label
				if selected {
					label = "* " + label
				} else {
					label = "  " + label
				}
				ebitenutil.DebugPrintAt(screen, label, menuTextX+menuSwatch+4, y+(menuItemH-12)/2)

			case menuKindCustomInput:
				// Live swatch from typed hex (or gray while incomplete).
				sw := color.RGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xFF}
				if hexColorRE.MatchString(normalizeColor(m.customHexInput)) {
					sw = colorSwatch(normalizeColor(m.customHexInput))
				}
				sx := float32(menuPad + 4)
				sy := float32(y + (menuItemH-menuSwatch)/2)
				vector.FillRect(screen, sx, sy, menuSwatch, menuSwatch, sw, false)
				vector.StrokeRect(screen, sx, sy, menuSwatch, menuSwatch, 1, color.RGBA{R: 200, G: 200, B: 200, A: 255}, false)

				// Text field: #RRGGBB with blinking cursor when focused.
				display := "#" + m.customHexInput
				if m.customInputFocus && (m.count/16)%2 == 0 {
					display += "_"
				} else if !m.customInputFocus && m.customHexInput == "" {
					display = "#______"
				}
				// Pad remaining slots lightly for fixed-width feel.
				ebitenutil.DebugPrintAt(screen, display, menuPad+4+menuSwatch+6, y+(menuItemH-12)/2)

			case menuKindCustomApply:
				label := "  Apply color"
				if disabled {
					label = "  Apply (need 6 hex)"
				} else if hovered {
					label = "> Apply color"
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+4, y+(menuItemH-12)/2)

			case menuKindAutostart:
				state := "Off"
				if autostartSupported && isAutostartEnabled() {
					state = "On"
				}
				label := "  Autostart: " + state
				if !autostartSupported {
					label = "  Autostart: n/a"
				} else if hovered {
					label = "> Autostart: " + state
				}
				if autostartSupported && isAutostartEnabled() {
					vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
						color.RGBA{R: 40, G: 60, B: 50, A: 255}, false)
					if hovered {
						vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
							color.RGBA{R: 50, G: 75, B: 60, A: 255}, false)
					}
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+4, y+(menuItemH-12)/2)

			case menuKindQuit:
				label := "  Quit"
				if hovered {
					label = "> Quit"
					vector.FillRect(screen, float32(menuPad), float32(y), float32(menuItemW), float32(menuItemH),
						color.RGBA{R: 70, G: 40, B: 40, A: 255}, false)
				}
				ebitenutil.DebugPrintAt(screen, label, menuPad+4, y+(menuItemH-12)/2)
			}

			y += menuItemH
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
		// Original body is white in the source sprites.
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
