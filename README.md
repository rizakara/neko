# Neko

![Neko](https://raw.githubusercontent.com/crgimenes/neko/master/assets/awake.png)

[Neko](https://en.wikipedia.org/wiki/Neko_(software)) is a cat that chases the mouse cursor across the screen, an app written in the late 1980s and ported for many platforms.

![Neco](https://github.com/crgimenes/neko/blob/master/fixtures/neko.gif)

This code is a re-implementation using Golang and **has no relationship to the original software**. Furthermore, this version does not use any part of the original source code except sprites and sounds.

The goal is to demonstrate the Go language with a fun and very nostalgic example, without necessarily being tied to the behavior of the original version.

In this example, we used the [Ebitengine](https://ebitengine.org), an incredibly easy-to-use gaming library with a vibrant community.

## Download (no Go required)

If you just want to use Neko, download a prebuilt binary from the
[latest release](https://github.com/crgimenes/neko/releases/latest). You do
**not** need Go or any developer tools.

Pick the file for your system:

| System | File to download |
| --- | --- |
| macOS (Intel or Apple Silicon) | `neko-darwin-universal.zip` |
| Windows (64-bit, most common) | `neko-windows-amd64.exe` |
| Windows (older 32-bit) | `neko-windows-386.exe` |
| Windows (ARM) | `neko-windows-arm64.exe` |
| Linux (Intel/AMD 64-bit) | `neko-linux-amd64.gz` |
| Linux (ARM 64-bit) | `neko-linux-arm64.gz` |

### macOS

1. Download `neko-darwin-universal.zip` and double-click it to unzip. You will get `neko.app`.
2. Move `neko.app` to your **Applications** folder (recommended).
3. Double-click `neko.app` to run it.

The app is signed and notarized by Apple, so it should open normally. The single
"universal" build works on both Intel and Apple Silicon Macs, so there is no need
to choose an architecture.

If macOS shows **"neko.app is damaged and can't be opened"** or warns about an
unidentified developer:

- Make sure the download finished completely, and unzip the file before opening
  it (do not run the app from inside the `.zip`). Re-download if in doubt.
- Right-click (or Control-click) `neko.app`, choose **Open**, then confirm with
  **Open** in the dialog.
- If it still will not open, open the **Terminal** app and run (adjust the path
  if you did not move it to Applications):

  ```bash
  xattr -dr com.apple.quarantine /Applications/neko.app
  ```

- Check **System Settings → Privacy & Security → Security** and make sure
  "Allow applications downloaded from" is set to **App Store & Known Developers**,
  not App Store only.

### Windows

Download the `.exe` for your system and double-click it. Windows SmartScreen may
warn you because the app is not from the Microsoft Store: click
**More info → Run anyway** to start it.

### Linux

Download the matching `.gz`, then decompress and run it:

```bash
gunzip neko-linux-amd64.gz
chmod +x neko-linux-amd64
./neko-linux-amd64
```

## Run from source

If you have Go installed and want to run Neko from source:

```bash
go run main.go
```

Or build the binary and run it:

```bash
go build -o neko main.go
```

On macOS and Windows no extra dependencies are needed. On **Linux**, Ebitengine
requires Cgo and the system development libraries, so build with
`CGO_ENABLED=1` after installing the packages listed in the
[Ebitengine install guide](https://ebitengine.org/en/documents/install.html)
(for example, on Debian/Ubuntu: `libgl1-mesa-dev`, `libasound2-dev`,
`libxcursor-dev`, `libxi-dev`, `libxinerama-dev`, `libxrandr-dev`,
`libxxf86vm-dev`, and `pkg-config`).
## Configuration

Neko is configured with a [Filo](https://github.com/crgimenes/filo) script. On
startup it looks for the config file in this order:

1. `./neko_init.filo` in the current directory.
2. `$XDG_CONFIG_HOME/neko/init.filo` (defaults to `~/.config/neko/init.filo`).

If no file is found, the built-in defaults are used. Any setting omitted from the
file keeps its default. See [`neko_init-sample.filo`](neko_init-sample.filo) for a
starting point:

```lisp
(do
  (set Speed 2.0)             ;; movement speed of the cat
  (set Scale 2.0)             ;; on-screen scale of the cat
  (set Quiet #f)              ;; #t disables sound
  (set MousePassthrough #f)   ;; #t lets clicks pass through the window
  (set SpriteSheet "")        ;; path to a custom sprite sheet; empty uses the built-in cat
  (set Color ""))             ;; built-in coat color as RRGGBB (e.g. "BC83DB"); empty is the original
```

Settings:

- `Speed` The speed of the cat (default 2.0).
- `Scale` The scale of the cat on the screen (default 2.0).
- `Quiet` Disable sound (default `#f`).
- `MousePassthrough` Enable mouse passthrough (default `#f`).
- `SpriteSheet` Path to a custom sprite sheet (default empty; see [Sprite sheets and skins](#sprite-sheets-and-skins)).
- `Color` Coat color as a 6-digit RRGGBB hex string (default empty = original). Presets: `BC83DB` Purple, `85B4DD` Blue, `EDAF71` Orange, `FFAAD4` Pink, `B1EA9D` Green, `7F7F7F` Gray — or any custom hex. Ignored when `SpriteSheet` is set.

Right-click the cat to open a **context menu** (fixed UI size; only the cat scales):

- **Size smaller / Size larger** — cat scale 1.0–8.0 (steps of 0.5).
- **Speed slower / Speed faster** — movement speed 0.5–10.0 (steps of 0.5; default 2.0).
- **Sound** — toggle sound effects on/off.
- **Color** — pick a built-in coat.
- **Custom** — type a 6-digit hex code (e.g. `FF00AA`) and **Apply color** (or
  Enter). The original white body is recolored at runtime; the coat is cached
  and appears in the Color list next time.
- **Autostart** — (Windows) run Neko at login for the current user via the
  registry `Run` key. Toggle on/off; uses the path of the `.exe` you launched.
- **Quit** — exit the program.

Escape or right-click again closes the menu. Size, sound, and typing keep the
menu open.

### Command-line flags

Flags override the config file (precedence: defaults < config file < flags):

- `-speed` The speed of the cat (default 2.0).
- `-scale` The scale of the cat on the screen (default 2.0).
- `-quiet` Disable sound.
- `-mousepassthrough` Enable mouse passthrough.
- `-spritesheet` Path to a custom oneko-layout sprite sheet (PNG).
- `-color` Built-in coat color as RRGGBB hex (e.g. `BC83DB`). Empty is the original; ignored with `-spritesheet`.
- `-h` Show help.

## Sprite sheets and skins

Neko draws its frames from a single sprite sheet: an 8x4 grid of 32x32 tiles
(256x128). The tile layout follows [adryd325/oneko.js](https://github.com/adryd325/oneko.js),
so sprite sheets from the oneko ecosystem can be loaded as skins.

Pass your own sheet with `-spritesheet path/to/sheet.png` (or the `SpriteSheet`
config setting). When no sheet is given, the built-in cat is used.

### Built-in coat colors

Source frames live in `sprites/` (plain names like `awake.png`). They use a
**white body + black outline**. At build time they pack into a single sheet
`assets/neko.png`. Preset and custom coats are produced at runtime by recoloring
that white body to a hex color (no per-color sprite files).

Presets: Original, Purple (`BC83DB`), Blue (`85B4DD`), Orange (`EDAF71`),
Pink (`FFAAD4`), Green (`B1EA9D`), Gray (`7F7F7F`).

To regenerate the sheet after editing sprites:

```bash
GENSHEET=1 go test -run TestGenerateSpriteSheet
```

## How to install

Before installing Neko, make sure you have Go installed on your system, as we will be using `go install`.

Install dependencies, build, and install the project into your Go bin directory:

```bash
cd neko
go mod tidy
go install
```

To use Neko globally across your system, check if your Go bin directory is in your `$PATH` by running:

```bash
echo $PATH
```
If you don't see a Go bin directory, you'll need to add the following line to your `~/.bashrc`, `~/.zshrc`, or the equivalent shell configuration file:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

After that, you can simply run `neko` in your terminal. If you want it to start with your system, you can add it to your `.xinitrc` or a similar startup script.

## How to Contribute

Please follow our [contribution guide](CONTRIBUTING.md).

---

## More of my projects

- [kutta](https://github.com/crgimenes/kutta): a 2D wind tunnel; watch air misbehave around an airfoil.
- [NeoFrame](https://github.com/crgimenes/NeoFrame): draw over your screen; a transparent overlay for demos and classes.
- [glaze](https://github.com/crgimenes/glaze): WebView desktop apps in Go, cgo-free.
- [filo](https://github.com/crgimenes/filo): a small scripting language safe to embed in Go programs.

More at [github.com/crgimenes](https://github.com/crgimenes) and [crg.eti.br](https://crg.eti.br).
