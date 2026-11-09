package typesafe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
)

const (
	prepareJPEGQuality = 85
	// maxDecodePixels caps what PrepareImage decodes: a few hundred bytes of
	// JPEG can declare 65535x65535 and force a multi-gigabyte allocation.
	maxDecodePixels = 50_000_000
)

// PrepareImage turns a PNG or JPEG photo into an Image the API accepts: it
// applies the JPEG EXIF orientation (the API ignores it), scales so each side
// lies within 280-1568 px keeping the aspect ratio, and re-encodes as JPEG.
// Transparent pixels become white. Photos more elongated than 1568:280, or
// over 50 megapixels, return ErrInvalidImage.
func PrepareImage(data []byte) (Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("%w: decode: %w", ErrInvalidImage, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxDecodePixels { // int64: the product overflows int on 32-bit
		return Image{}, fmt.Errorf("%w: %dx%d is over %d pixels", ErrInvalidImage, cfg.Width, cfg.Height, maxDecodePixels)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("%w: decode: %w", ErrInvalidImage, err)
	}
	oriented := orient(onWhite(src), exifOrientation(data))
	b := oriented.Bounds()
	w, h, err := fitSize(b.Dx(), b.Dy())
	if err != nil {
		return Image{}, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scale(oriented, w, h), &jpeg.Options{Quality: prepareJPEGQuality}); err != nil {
		return Image{}, fmt.Errorf("typesafe: encode jpeg: %w", err)
	}
	return NewImage(buf.Bytes()), nil
}

// onWhite flattens images with an alpha channel onto white; JPEG has none and
// would otherwise turn transparent pixels black.
func onWhite(src image.Image) image.Image {
	if o, ok := src.(interface{ Opaque() bool }); ok && o.Opaque() {
		return src
	}
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
	return dst
}

// fitSize returns the size, aspect ratio kept, that puts both sides within the
// API bounds, shrinking before growing.
func fitSize(w, h int) (int, int, error) {
	long, short := float64(max(w, h)), float64(min(w, h))
	f := min(1, maxImageSide/long)
	if short*f < minImageSide {
		f = minImageSide / short
	}
	nw := min(max(int(math.Round(float64(w)*f)), minImageSide), maxImageSide)
	nh := min(max(int(math.Round(float64(h)*f)), minImageSide), maxImageSide)
	if long*f > maxImageSide+0.5 {
		return 0, 0, fmt.Errorf("%w: %dx%d is too elongated to fit %d-%d px per side",
			ErrInvalidImage, w, h, minImageSide, maxImageSide)
	}
	return nw, nh, nil
}

// scale resizes by averaging the source pixels under each destination pixel
// (nearest pixel when enlarging).
// ponytail: plain area average, swap for golang.org/x/image/draw if quality matters.
func scale(src image.Image, w, h int) image.Image {
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0 := b.Min.Y + y*b.Dy()/h
		y1 := max(b.Min.Y+(y+1)*b.Dy()/h, y0+1)
		for x := range w {
			x0 := b.Min.X + x*b.Dx()/w
			x1 := max(b.Min.X+(x+1)*b.Dx()/w, x0+1)
			var r, g, bl, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, bl, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 0xff})
		}
	}
	return dst
}

// orient redraws src upright for EXIF orientation o (1-8); 1 or unknown
// values return src unchanged.
func orient(src image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 { // 5-8 swap width and height
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2: // mirror horizontally
				sx, sy = w-1-x, y
			case 3: // rotate 180
				sx, sy = w-1-x, h-1-y
			case 4: // mirror vertically
				sx, sy = x, h-1-y
			case 5: // transpose
				sx, sy = y, x
			case 6: // rotate 90 clockwise
				sx, sy = y, h-1-x
			case 7: // transverse
				sx, sy = w-1-y, h-1-x
			case 8: // rotate 90 counter-clockwise
				sx, sy = w-1-y, x
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// exifOrientation reads tag 0x0112 from a JPEG's APP1 Exif segment, returning
// 1 (upright) when the data is not a JPEG or carries no readable tag.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(data) && data[i] == 0xFF; {
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // image data or end: no Exif before it
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		end := i + 2 + size
		if size < 2 || end > len(data) {
			return 1
		}
		if seg := data[i+4 : end]; marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i = end
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(t[4:]))
	if ifd < 8 || ifd > len(t)-2 { // subtraction side: ifd+2 can overflow int on 32-bit
		return 1
	}
	count := int(order.Uint16(t[ifd:]))
	for e := range count {
		at := ifd + 2 + e*12
		if at+12 > len(t) {
			return 1
		}
		if order.Uint16(t[at:]) == 0x0112 {
			return int(order.Uint16(t[at+8:]))
		}
	}
	return 1
}
