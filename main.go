package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/crgimenes/filo"
)

type psinoza struct {
	waiting    bool
	x          float64
	y          float64
	count      int
	min        int
	max        int
	state      int
	sprite     string
	lastSprite string
	img        *ebiten.Image
	monitor    *ebiten.MonitorType

	cfg         *Config
	sprites     map[string]*ebiten.Image
	// baseSheet is the original white-body sheet (CPU image) used to recolor
	// coats without calling ebiten.ReadPixels (which panics before RunGame).
	baseSheet *image.NRGBA
	// colorSheets holds every built-in color variant (key "" is the original).
	// When a custom SpriteSheet is set, only the active color is populated.
	// Runtime custom hex coats are also cached here after recoloring.
	colorSheets map[string]map[string]*ebiten.Image
	colorOrder  []string // coats listed in the color menu (built-in + customs)
	color       string   // active color key; "" is the original cat

	menuOpen         bool        // context menu is showing
	menuHover        int         // hovered menu row, or -1
	menuHoverColor   int         // hovered color swatch index, or -1
	menuScale        float64     // window scale fitted to current monitor
	menuEntries      []menuEntry // flat list of rows for the open menu
	customHexInput   string      // typed RRGGBB buffer for custom color
	customInputFocus bool        // hex field is capturing keyboard

	sounds        map[string][]byte
	audioContext  *audio.Context
	currentPlayer *audio.Player
}

type Config struct {
	Speed            float64
	Scale            float64
	Quiet            bool
	MousePassthrough bool
	SpriteSheet      string
	// Color selects a built-in coat color by RRGGBB hex (e.g. "BC83DB").
	// Empty uses the original sprites. Ignored when SpriteSheet is set.
	Color string
}

type movementDirection int

const (
	directionRight movementDirection = iota
	directionDownRight
	directionDown
	directionDownLeft
	directionLeft
	directionUpLeft
	directionUp
	directionUpRight
	directionCount
)

const (
	width                = 32
	height               = 32
	sampleRate           = 44100
	soundVolume          = 0.3
	directionSectorAngle = 45.0
	directionHalfSector  = directionSectorAngle / 2
	arrivalSlowdownRange = width * 3
	// Disabled by default: a cat should keep its speed while pouncing.
	arrivalSlowdownEnabled = false
	// A small overlap keeps the animation stable without changing movement.
	directionHysteresis = 3.0
)

// Idle animation timeline. While the cat is idle, m.state advances through
// these phases on each animation cycle; a phase spans every tick from its
// threshold up to the next one. stateMoving means the cat is chasing the
// cursor rather than idling.
const (
	stateMoving     = 0
	stateAwakeStart = 1
	stateScratch    = 4
	stateWash       = 7
	stateYawn       = 10
	stateSleep      = 13
)

// spriteCell is the column and row of a 32px tile inside a sprite sheet.
type spriteCell struct {
	col int
	row int
}

// The sprite sheet is an 8x4 grid of 32px tiles (256x128), following the
// layout used by adryd325/oneko.js (https://github.com/adryd325/oneko.js).
// Sharing that layout lets third-party oneko sheets be loaded as skins.
const (
	sheetColumns = 8
	sheetRows    = 4
)

// spriteSheetLayout maps each frame Psinoza draws to its cell in the sheet. The
// eight movement directions and the sleeping frames sit on their canonical
// oneko cells so foreign oneko sheets animate correctly; the remaining idle
// frames reuse the nearest oneko state, since Psinoza has a few states oneko does
// not. Cells absent from this map are unused and may be left transparent.
var spriteSheetLayout = map[string]spriteCell{
	// Movement directions, two frames each (oneko N, NE, E, SE, S, SW, W, NW).
	"up1":        {1, 2},
	"up2":        {1, 3},
	"upright1":   {0, 2},
	"upright2":   {0, 3},
	"right1":     {3, 0},
	"right2":     {3, 1},
	"downright1": {5, 1},
	"downright2": {5, 2},
	"down1":      {6, 3},
	"down2":      {7, 2},
	"downleft1":  {5, 3},
	"downleft2":  {6, 1},
	"left1":      {4, 2},
	"left2":      {4, 3},
	"upleft1":    {1, 0},
	"upleft2":    {1, 1},

	// Idle states.
	"awake":    {7, 3}, // oneko "alert"
	"sleep1":   {2, 0}, // oneko "sleeping"
	"sleep2":   {2, 1},
	"yawn1":    {3, 2}, // oneko "tired"
	"yawn2":    {3, 3}, // oneko "idle" (oneko has no second tired frame)
	"scratch1": {5, 0}, // oneko "scratchSelf"
	"scratch2": {6, 0},
	"wash":     {7, 0}, // oneko "scratchSelf" (third frame); single frame, since
	//                     oneko grooming is the closest match to washing and
	//                     only three scratchSelf cells exist.
}

var (
	// The generated default sprite sheet and sounds ship in the binary.
	// Individual source sprites under sprites/ are build-time inputs for the
	// generator (see gensheet_test.go). Built-in coat colors are recolored at
	// runtime from the original white body (same path as custom hex).
	//go:embed assets/psinoza.png
	//go:embed assets/*.wav
	embeddedFS embed.FS

	// hexColorRE validates a 6-digit RRGGBB color token (no leading #).
	hexColorRE = regexp.MustCompile(`(?i)^[0-9A-Fa-f]{6}$`)
)

// presetCoatColors are the built-in menu coats (excluding original "").
// Each is generated by recoloring the white body of assets/psinoza.png.
var presetCoatColors = []string{
	"BC83DB", // purple
	"85B4DD", // blue
	"EDAF71", // orange
	"FFAAD4", // pink
	"B1EA9D", // green
	"7F7F7F", // gray
}

// normalizeColor uppercases a hex color token. Empty input stays empty (default).
func normalizeColor(color string) string {
	color = strings.TrimSpace(color)
	color = strings.TrimPrefix(color, "#")
	if color == "" {
		return ""
	}
	return strings.ToUpper(color)
}

func (m *psinoza) Layout(outsideWidth, outsideHeight int) (int, int) {
	if m.menuOpen {
		return m.menuSize()
	}
	return width, height
}

func (m *psinoza) playSound(soundName string) {
	if m.cfg.Quiet {
		return
	}
	sound, ok := m.sounds[soundName]
	if !ok {
		return
	}
	if m.currentPlayer != nil {
		_ = m.currentPlayer.Close()
		m.currentPlayer = nil
	}
	m.currentPlayer = m.audioContext.NewPlayerFromBytes(sound)
	m.currentPlayer.SetVolume(soundVolume)
	m.currentPlayer.Play()
}

func angularDistance(a, b float64) float64 {
	distance := math.Abs(a - b)
	return min(distance, 360-distance)
}

func movementVector(x, y, speed float64, slowdown bool) (float64, float64) {
	distance := math.Hypot(x, y)
	if distance == 0 || speed <= 0 {
		return 0, 0
	}

	if slowdown && distance < arrivalSlowdownRange {
		speed *= distance / arrivalSlowdownRange
	}
	speed = min(speed, distance)

	scale := speed / distance
	return x * scale, y * scale
}

func parseMovementDirection(sprite string) (movementDirection, bool) {
	switch sprite {
	case "right":
		return directionRight, true
	case "downright":
		return directionDownRight, true
	case "down":
		return directionDown, true
	case "downleft":
		return directionDownLeft, true
	case "left":
		return directionLeft, true
	case "upleft":
		return directionUpLeft, true
	case "up":
		return directionUp, true
	case "upright":
		return directionUpRight, true
	default:
		return 0, false
	}
}

func (d movementDirection) sprite() string {
	switch d {
	case directionRight:
		return "right"
	case directionDownRight:
		return "downright"
	case directionDown:
		return "down"
	case directionDownLeft:
		return "downleft"
	case directionLeft:
		return "left"
	case directionUpLeft:
		return "upleft"
	case directionUp:
		return "up"
	case directionUpRight:
		return "upright"
	default:
		return ""
	}
}

func spriteDirection(angle float64, previous string) string {
	if direction, ok := parseMovementDirection(previous); ok {
		center := float64(direction) * directionSectorAngle
		if angularDistance(angle, center) <= directionHalfSector+directionHysteresis {
			return previous
		}
	}

	index := int(math.Ceil((angle - directionHalfSector) / directionSectorAngle))
	index = (index + int(directionCount)) % int(directionCount)
	return movementDirection(index).sprite()
}

func (m *psinoza) syncMonitor(
	current *ebiten.MonitorType,
	windowPosition func() (int, int),
) {
	if current == nil {
		return
	}
	if m.monitor == nil {
		m.monitor = current
		return
	}
	if current == m.monitor {
		return
	}

	x, y := windowPosition()
	m.x = float64(x)
	m.y = float64(y)
	m.monitor = current
}

func (m *psinoza) Update() error {
	m.count++

	// Color menu captures all input until closed.
	if m.menuOpen {
		return m.updateColorMenu()
	}
	if m.state == stateYawn && m.count == m.min {
		m.playSound("idle3")
	}

	// Window coordinates are relative to the current monitor. Rebase the
	// internal position when the window crosses into another monitor.
	m.syncMonitor(ebiten.Monitor(), ebiten.WindowPosition)
	ebiten.SetWindowPosition(int(math.Round(m.x)), int(math.Round(m.y)))

	mx, my := ebiten.CursorPositionF()
	x := mx - (width / 2)
	y := my - (height / 2)

	distance := math.Abs(x) + math.Abs(y)
	if distance < width || m.waiting {
		m.stayIdle()
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			m.waiting = !m.waiting
		}
		// Right-click opens the context menu (size + colors).
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			m.openColorMenu()
		}
		return nil
	}

	if m.state >= stateSleep {
		m.playSound("awake")
	}
	m.catchCursor(x, y)
	return nil
}

func (m *psinoza) stayIdle() {
	switch {
	case m.state < stateScratch:
		if m.state == stateMoving {
			m.state = stateAwakeStart
		}
		m.sprite = "awake"

	case m.state < stateWash:
		m.sprite = "scratch"

	case m.state < stateYawn:
		m.sprite = "wash"

	case m.state < stateSleep:
		m.min = 32
		m.max = 64
		m.sprite = "yawn"

	default:
		m.sprite = "sleep"
	}
}

func (m *psinoza) catchCursor(x, y float64) {
	m.state = 0
	m.min = 8
	m.max = 16

	// get mouse direction
	r := math.Atan2(y, x)
	a := math.Mod((r/math.Pi*180)+360, 360) // Normazing angle to [0, 360)
	m.sprite = spriteDirection(a, m.sprite)

	dx, dy := movementVector(x, y, m.cfg.Speed, arrivalSlowdownEnabled)
	m.x += dx
	m.y += dy
}

func (m *psinoza) Draw(screen *ebiten.Image) {
	if m.menuOpen {
		m.drawColorMenu(screen)
		return
	}

	var sprite string
	switch {
	case m.sprite == "awake" || m.sprite == "wash":
		sprite = m.sprite
	case m.count < m.min:
		sprite = m.sprite + "1"
	default:
		sprite = m.sprite + "2"
	}

	m.img = m.sprites[sprite]

	if m.count > m.max {
		m.count = 0

		if m.state > stateMoving {
			m.state++
			switch m.state {
			case stateSleep:
				m.playSound("sleep")
			}
		}
	}

	if m.lastSprite == sprite {
		return
	}

	m.lastSprite = sprite

	screen.Clear()

	screen.DrawImage(m.img, nil)
}

// subImager is implemented by the standard library image types that back a
// decoded PNG, allowing zero-copy crops of a sprite sheet.
type subImager interface {
	SubImage(r image.Rectangle) image.Image
}

// loadSpriteSheet returns the raw PNG bytes of the sprite sheet to use: the
// user-provided file when sheetPath is set, otherwise the embedded original
// cat (coat colors are applied later via recolorSprites).
func loadSpriteSheet(sheetPath, color string) ([]byte, error) {
	_ = color // color variants are no longer separate embedded sheets
	if sheetPath != "" {
		data, err := os.ReadFile(filepath.Clean(sheetPath))
		if err != nil {
			return nil, fmt.Errorf("read sprite sheet %q: %w", sheetPath, err)
		}
		return data, nil
	}
	const asset = "assets/psinoza.png"
	data, err := embeddedFS.ReadFile(asset)
	if err != nil {
		return nil, fmt.Errorf("read embedded sheet %q: %w", asset, err)
	}
	return data, nil
}

// decodeSheetNRGBA decodes a PNG sprite sheet into a mutable NRGBA image.
func decodeSheetNRGBA(sheet []byte) (*image.NRGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(sheet))
	if err != nil {
		return nil, fmt.Errorf("decode sprite sheet: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() < sheetColumns*width || bounds.Dy() < sheetRows*height {
		return nil, fmt.Errorf(
			"sprite sheet is %dx%d, need at least %dx%d",
			bounds.Dx(), bounds.Dy(), sheetColumns*width, sheetRows*height,
		)
	}
	return imageToNRGBA(img), nil
}

// imageToNRGBA returns a dense NRGBA copy of img (safe to mutate).
func imageToNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// loadSprites decodes an oneko-layout sprite sheet and slices it into the
// individual frames Psinoza draws.
func loadSprites(sheet []byte) (map[string]*ebiten.Image, error) {
	nrgba, err := decodeSheetNRGBA(sheet)
	if err != nil {
		return nil, err
	}
	return spritesFromNRGBA(nrgba), nil
}

// spritesFromNRGBA crops each frame from a sheet into ebiten images.
// Uses NewImageFromImage only (safe before RunGame); never ReadPixels.
func spritesFromNRGBA(sheet *image.NRGBA) map[string]*ebiten.Image {
	bounds := sheet.Bounds()
	sprites := make(map[string]*ebiten.Image, len(spriteSheetLayout))
	for name, cell := range spriteSheetLayout {
		x0 := bounds.Min.X + cell.col*width
		y0 := bounds.Min.Y + cell.row*height
		rect := image.Rect(x0, y0, x0+width, y0+height)
		sprites[name] = ebiten.NewImageFromImage(sheet.SubImage(rect))
	}
	return sprites
}

// defaultColorOrder returns original ("") plus the named preset coats.
func defaultColorOrder() []string {
	order := make([]string, 0, 1+len(presetCoatColors))
	order = append(order, "")
	order = append(order, presetCoatColors...)
	return order
}

// loadBuiltinColorSheets loads the original sheet and pre-generates preset
// coats by recoloring the white body on the CPU (no ebiten.ReadPixels).
func loadBuiltinColorSheets() (map[string]map[string]*ebiten.Image, []string, *image.NRGBA, error) {
	data, err := loadSpriteSheet("", "")
	if err != nil {
		return nil, nil, nil, err
	}
	baseSheet, err := decodeSheetNRGBA(data)
	if err != nil {
		return nil, nil, nil, err
	}

	order := defaultColorOrder()
	sheets := make(map[string]map[string]*ebiten.Image, len(order))
	sheets[""] = spritesFromNRGBA(baseSheet)
	for _, hex := range presetCoatColors {
		sheets[hex] = spritesFromNRGBA(recolorNRGBA(baseSheet, colorSwatch(hex)))
	}
	return sheets, order, baseSheet, nil
}

// setColor switches the active coat. Returns false if the color is unknown
// and cannot be generated by recoloring the original white body.
func (m *psinoza) setColor(color string) bool {
	color = normalizeColor(color)
	if sprites, ok := m.colorSheets[color]; ok {
		m.color = color
		m.sprites = sprites
		m.cfg.Color = color
		m.lastSprite = ""
		return true
	}
	// Unknown hex: try runtime recolor from the original sheet.
	if color != "" && hexColorRE.MatchString(color) {
		return m.applyCustomColor(color)
	}
	return false
}

// applyCustomColor builds a coat by painting the original white body with hex,
// caches it, and activates it. Works because built-in source frames are pure
// white + black silhouettes.
func (m *psinoza) applyCustomColor(hex string) bool {
	hex = normalizeColor(hex)
	if !hexColorRE.MatchString(hex) {
		return false
	}
	if sprites, ok := m.colorSheets[hex]; ok {
		m.color = hex
		m.sprites = sprites
		m.cfg.Color = hex
		m.lastSprite = ""
		return true
	}
	if m.baseSheet == nil {
		return false
	}
	sprites := spritesFromNRGBA(recolorNRGBA(m.baseSheet, colorSwatch(hex)))
	m.colorSheets[hex] = sprites
	// Append to menu list once so the custom coat is pickable later.
	found := false
	for _, c := range m.colorOrder {
		if c == hex {
			found = true
			break
		}
	}
	if !found {
		m.colorOrder = append(m.colorOrder, hex)
	}
	m.color = hex
	m.sprites = sprites
	m.cfg.Color = hex
	m.lastSprite = ""
	return true
}

// recolorNRGBA returns a copy of src with near-white body pixels painted target.
// Pure image/ CPU path — safe before ebiten.RunGame (unlike Image.ReadPixels).
func recolorNRGBA(src *image.NRGBA, target color.RGBA) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	copy(dst.Pix, src.Pix)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		off := dst.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			i := off + (x-b.Min.X)*4
			if dst.Pix[i+3] == 0 {
				continue
			}
			// Original body fill is pure white; keep outlines (black) as-is.
			if dst.Pix[i] >= 200 && dst.Pix[i+1] >= 200 && dst.Pix[i+2] >= 200 {
				dst.Pix[i] = target.R
				dst.Pix[i+1] = target.G
				dst.Pix[i+2] = target.B
			}
		}
	}
	return dst
}

// loadSounds decodes the embedded .wav assets into raw PCM byte slices keyed by
// file name without extension.
func loadSounds(assetsFS fs.FS, sampleRate int) (map[string][]byte, error) {
	sounds := make(map[string][]byte)

	entries, err := fs.ReadDir(assetsFS, "assets")
	if err != nil {
		return nil, fmt.Errorf("read assets directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".wav" {
			continue
		}

		assetPath := path.Join("assets", entry.Name())
		data, err := fs.ReadFile(assetsFS, assetPath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", assetPath, err)
		}

		stream, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode sound %q: %w", entry.Name(), err)
		}
		soundData, err := io.ReadAll(stream)
		if err != nil {
			return nil, fmt.Errorf("read sound %q: %w", entry.Name(), err)
		}

		name := strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		sounds[name] = soundData
	}

	return sounds, nil
}

// configPath returns the path to the Filo config file. A local "psinoza_init.filo"
// in the current directory takes precedence; otherwise the XDG config location
// "$XDG_CONFIG_HOME/psinoza/init.filo" (defaulting to ~/.config/psinoza/init.filo) is
// used.
func configPath() string {
	const local = "psinoza_init.filo"
	if fileExists(local) {
		return local
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return local
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "psinoza", "init.filo")
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

// loadConfig builds the configuration from defaults, overriding them with any
// values set in the Filo config file (if present).
func loadConfig() *Config {
	cfg := &Config{
		Speed:            2.0,
		Scale:            2.0,
		Quiet:            false,
		MousePassthrough: false,
		SpriteSheet:      "",
		Color:            "",
	}

	name := configPath()
	if !fileExists(name) {
		return cfg
	}

	f := filo.New()
	defer f.Close()

	f.SetGlobal("Speed", cfg.Speed)
	f.SetGlobal("Scale", cfg.Scale)
	f.SetGlobal("Quiet", cfg.Quiet)
	f.SetGlobal("MousePassthrough", cfg.MousePassthrough)
	f.SetGlobal("SpriteSheet", cfg.SpriteSheet)
	f.SetGlobal("Color", cfg.Color)

	b, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		log.Fatal(err)
	}
	if err := f.DoString(string(b)); err != nil {
		log.Fatal(err)
	}

	cfg.Speed = f.MustGetNumber("Speed")
	cfg.Scale = f.MustGetNumber("Scale")
	cfg.Quiet = f.MustGetBool("Quiet")
	cfg.MousePassthrough = f.MustGetBool("MousePassthrough")
	cfg.SpriteSheet = f.MustGetString("SpriteSheet")
	cfg.Color = normalizeColor(f.MustGetString("Color"))

	return cfg
}

func main() {
	cfg := loadConfig()

	// Command-line flags override the Filo config file. Their defaults are the
	// values loaded above, so a flag only takes effect when explicitly passed.
	flag.Float64Var(&cfg.Speed, "speed", cfg.Speed, "The speed of the cat.")
	flag.Float64Var(&cfg.Scale, "scale", cfg.Scale, "The scale of the cat on the screen.")
	flag.BoolVar(&cfg.Quiet, "quiet", cfg.Quiet, "Disable sound.")
	flag.BoolVar(&cfg.MousePassthrough, "mousepassthrough", cfg.MousePassthrough, "Enable mouse passthrough.")
	flag.StringVar(&cfg.SpriteSheet, "spritesheet", cfg.SpriteSheet, "Path to a custom oneko-layout sprite sheet (PNG).")
	flag.StringVar(&cfg.Color, "color", cfg.Color, "Built-in coat color as RRGGBB hex (e.g. BC83DB). Empty is the original; ignored with -spritesheet.")
	flag.Parse()
	cfg.Color = normalizeColor(cfg.Color)

	var (
		colorSheets map[string]map[string]*ebiten.Image
		colorOrder  []string
		sprites     map[string]*ebiten.Image
		baseSheet   *image.NRGBA
		err         error
	)

	if cfg.SpriteSheet != "" {
		// Custom sheet: single skin, color cycling disabled.
		sheet, err := loadSpriteSheet(cfg.SpriteSheet, "")
		if err != nil {
			log.Fatal(err)
		}
		baseSheet, err = decodeSheetNRGBA(sheet)
		if err != nil {
			log.Fatal(err)
		}
		sprites = spritesFromNRGBA(baseSheet)
		colorSheets = map[string]map[string]*ebiten.Image{"": sprites}
		colorOrder = []string{""}
		cfg.Color = ""
	} else {
		colorSheets, colorOrder, baseSheet, err = loadBuiltinColorSheets()
		if err != nil {
			log.Fatal(err)
		}
		if cfg.Color != "" && colorSheets[cfg.Color] == nil {
			// Any valid RRGGBB is allowed: recolor the original white body at runtime.
			if !hexColorRE.MatchString(cfg.Color) {
				log.Fatalf("unknown color %q (use RRGGBB hex, or a preset: %s)", cfg.Color, strings.Join(colorOrder[1:], ", "))
			}
			colorSheets[cfg.Color] = spritesFromNRGBA(recolorNRGBA(baseSheet, colorSwatch(cfg.Color)))
			colorOrder = append(colorOrder, cfg.Color)
		}
		sprites = colorSheets[cfg.Color]
	}

	sounds, err := loadSounds(embeddedFS, sampleRate)
	if err != nil {
		log.Fatal(err)
	}

	audioContext := audio.NewContext(sampleRate)

	// Workaround: for some reason playing the first sound can incur significant delay.
	// So let's do this at the start.
	audioContext.NewPlayerFromBytes([]byte{}).Play()

	monitorWidth, monitorHeight := ebiten.Monitor().Size()

	n := &psinoza{
		x:            float64(monitorWidth / 2),
		y:            float64(monitorHeight / 2),
		min:          8,
		max:          16,
		cfg:          cfg,
		sprites:      sprites,
		baseSheet:    baseSheet,
		colorSheets:  colorSheets,
		colorOrder:   colorOrder,
		color:        cfg.Color,
		sounds:       sounds,
		audioContext: audioContext,
	}
	if cfg.Color != "" {
		n.customHexInput = cfg.Color
	}

	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetTPS(50)
	ebiten.SetVsyncEnabled(true)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(cfg.MousePassthrough)
	ebiten.SetWindowSize(int(float64(width)*cfg.Scale), int(float64(height)*cfg.Scale))
	ebiten.SetWindowTitle("Psinoza")

	err = ebiten.RunGameWithOptions(n, &ebiten.RunGameOptions{
		InitUnfocused:     true,
		ScreenTransparent: true,
		SkipTaskbar:       true,
		X11ClassName:      "Psinoza",
		X11InstanceName:   "Psinoza",
	})
	if err != nil {
		log.Fatal(err)
	}
}
