package typesafe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
)

// withOrientation splices an APP1 Exif segment holding orientation o right
// after the JPEG's SOI marker, as a camera would write it.
func withOrientation(data []byte, order binary.ByteOrder, o uint16) []byte {
	tiff := make([]byte, 8+2+12+4)
	if order == binary.LittleEndian {
		copy(tiff, "II")
	} else {
		copy(tiff, "MM")
	}
	order.PutUint16(tiff[2:], 42)
	order.PutUint32(tiff[4:], 8)
	order.PutUint16(tiff[8:], 1)       // one entry
	order.PutUint16(tiff[10:], 0x0112) // Orientation
	order.PutUint16(tiff[12:], 3)      // SHORT
	order.PutUint32(tiff[14:], 1)
	order.PutUint16(tiff[18:], o)
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(seg)+2))
	out := append([]byte{}, data[:2]...)
	out = append(out, app1...)
	out = append(out, seg...)
	return append(out, data[2:]...)
}

// marked is a w x h white image with a red block in its top-left corner.
func marked(w, h int) image.Image {
	img := solid(w, h, color.White)
	for y := range h / 4 {
		for x := range w / 4 {
			img.Set(x, y, color.RGBA{0xff, 0, 0, 0xff})
		}
	}
	return img
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xc000 && g < 0x4000 && b < 0x4000
}

func decoded(t *testing.T, img Image) image.Image {
	t.Helper()
	out, format, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil || format != "jpeg" {
		t.Fatalf("decode: %v format %q", err, format)
	}
	if img.Ref == "" {
		t.Error("empty Ref")
	}
	return out
}

func TestPrepareImage_appliesOrientation(t *testing.T) {
	// The stored pixels are 400x300 with red top-left; each orientation says
	// where that corner belongs when shown upright.
	cases := []struct {
		o          uint16
		order      binary.ByteOrder
		w, h       int
		redX, redY int // fractions of the upright size, in quarters
	}{
		{1, binary.BigEndian, 400, 300, 0, 0},
		{3, binary.LittleEndian, 400, 300, 3, 3},
		{6, binary.BigEndian, 300, 400, 3, 0},
		{8, binary.LittleEndian, 300, 400, 0, 3},
		{2, binary.BigEndian, 400, 300, 3, 0},
		{4, binary.BigEndian, 400, 300, 0, 3},
		{5, binary.BigEndian, 300, 400, 0, 0},
		{7, binary.BigEndian, 300, 400, 3, 3},
	}
	src := jpegBytes(t, marked(400, 300))
	for _, tc := range cases {
		img, err := PrepareImage(withOrientation(src, tc.order, tc.o))
		if err != nil {
			t.Fatalf("orientation %d: %v", tc.o, err)
		}
		out := decoded(t, img)
		b := out.Bounds()
		if b.Dx() != tc.w || b.Dy() != tc.h {
			t.Errorf("orientation %d: size %v, want %dx%d", tc.o, b.Size(), tc.w, tc.h)
			continue
		}
		x, y := tc.redX*tc.w/4+tc.w/8, tc.redY*tc.h/4+tc.h/8
		if !isRed(out.At(x, y)) {
			t.Errorf("orientation %d: (%d,%d) is %v, want red", tc.o, x, y, out.At(x, y))
		}
	}
}

func TestPrepareImage_scalesIntoBounds(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		w, h int
	}{
		{"large jpeg shrinks", jpegBytes(t, marked(4000, 3000)), 1568, 1176},
		{"small png grows", pngBytes(t, marked(100, 200)), 280, 560},
		{"in bounds keeps size", jpegBytes(t, marked(800, 600)), 800, 600},
		{"long but fits", jpegBytes(t, marked(3000, 600)), 1568, 314},
	}
	for _, tc := range cases {
		img, err := PrepareImage(tc.data)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		out := decoded(t, img)
		if s := out.Bounds().Size(); s.X != tc.w || s.Y != tc.h {
			t.Errorf("%s: size %v, want %dx%d", tc.name, s, tc.w, tc.h)
		}
		if !isRed(out.At(tc.w/16, tc.h/16)) {
			t.Errorf("%s: top-left lost its colour", tc.name)
		}
		if _, err := img.validate(); err != nil {
			t.Errorf("%s: prepared image fails validation: %v", tc.name, err)
		}
	}
}

func TestPrepareImage_rejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"too elongated": pngBytes(t, solid(2000, 300, color.White)),
		"not an image":  []byte("nope"),
	} {
		if _, err := PrepareImage(data); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestExifOrientation_malformed(t *testing.T) {
	ok := jpegBytes(t, solid(10, 10, color.White))
	truncated := withOrientation(ok, binary.BigEndian, 6)[:20]
	exif := func(tiff []byte) []byte {
		seg := append([]byte("Exif\x00\x00"), tiff...)
		app1 := []byte{0xFF, 0xE1, 0, 0}
		binary.BigEndian.PutUint16(app1[2:], uint16(len(seg)+2))
		return append(append(append([]byte{0xFF, 0xD8}, app1...), seg...), ok[2:]...)
	}
	tiff := func(ifd uint32, count uint16) []byte {
		t := []byte{'M', 'M', 0, 42, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(t[4:], ifd)
		binary.BigEndian.PutUint16(t[8:], count)
		return t
	}
	for name, data := range map[string][]byte{
		"png":                  pngBytes(t, solid(10, 10, color.White)),
		"no exif":              ok,
		"truncated":            truncated,
		"empty":                nil,
		"ifd past the end":     exif(tiff(4096, 1)),
		"ifd near max":         exif(tiff(0xFFFFFFFF, 1)),
		"huge entry count":     exif(tiff(8, 0xFFFF)),
		"segment size under 2": {0xFF, 0xD8, 0xFF, 0xE1, 0, 1, 'E', 'x'},
	} {
		if got := exifOrientation(data); got != 1 {
			t.Errorf("%s: %d", name, got)
		}
	}
}

func TestPrepareImage_rejectsHugeDeclaredSize(t *testing.T) {
	data := jpegBytes(t, solid(10, 10, color.White))
	sof := bytes.Index(data, []byte{0xFF, 0xC0})
	if sof < 0 {
		t.Fatal("no SOF0 marker")
	}
	binary.BigEndian.PutUint16(data[sof+5:], 65535) // height
	binary.BigEndian.PutUint16(data[sof+7:], 65535) // width
	if _, err := PrepareImage(data); !errors.Is(err, ErrInvalidImage) || !strings.Contains(err.Error(), "65535x65535") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrepareImage_flattensTransparencyOnWhite(t *testing.T) {
	img, err := PrepareImage(pngBytes(t, image.NewNRGBA(image.Rect(0, 0, 300, 300))))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decoded(t, img).At(150, 150).RGBA()
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Errorf("transparent pixel became %v, want white", decoded(t, img).At(150, 150))
	}
}
