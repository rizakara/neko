package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// spritePNGName returns the individual sprite filename for a frame.
// "wash" is drawn from wash1.png.
func spritePNGName(frame string) string {
	if frame == "wash" {
		return "wash1.png"
	}
	return frame + ".png"
}

// composeSpriteSheet builds an oneko-layout sheet from sprites/.
func composeSpriteSheet(spritesDir string) (*image.NRGBA, error) {
	sheet := image.NewNRGBA(image.Rect(0, 0, sheetColumns*width, sheetRows*height))

	for name, cell := range spriteSheetLayout {
		src := filepath.Join(spritesDir, spritePNGName(name))
		f, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		img, err := png.Decode(f)
		_ = f.Close() // read-only; a close error cannot corrupt anything
		if err != nil {
			return nil, err
		}

		dst := image.Rect(
			cell.col*width,
			cell.row*height,
			cell.col*width+width,
			cell.row*height+height,
		)
		draw.Draw(sheet, dst, img, img.Bounds().Min, draw.Src)
	}

	return sheet, nil
}

// TestGenerateSpriteSheet composes the individual sprite PNGs in sprites/ into
// a single oneko-layout sheet at assets/neko.png. Built-in coat colors are no
// longer packed as separate sheets; they are recolored at runtime from this
// original white body. Cells not present in spriteSheetLayout are left
// transparent. It is a generator rather than a check, so it only runs when
// GENSHEET is set:
//
//	GENSHEET=1 go test -run TestGenerateSpriteSheet
func TestGenerateSpriteSheet(t *testing.T) {
	if os.Getenv("GENSHEET") == "" {
		t.Skip("set GENSHEET=1 to regenerate assets/neko.png")
	}

	sheet, err := composeSpriteSheet("sprites")
	if err != nil {
		t.Fatalf("compose sheet: %v", err)
	}

	out := filepath.Join("assets", "neko.png")
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("create %s: %v", out, err)
	}
	if err := png.Encode(f, sheet); err != nil {
		_ = f.Close()
		t.Fatalf("encode %s: %v", out, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", out, err)
	}
	t.Logf("wrote %s (%d sprites)", out, len(spriteSheetLayout))
}
