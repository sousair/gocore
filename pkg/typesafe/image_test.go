package typesafe

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// boxBody is the Box response example from docs.typesafe.ai/primitives/box.
const boxBody = `{"model":"jev-1.14.0","answers":{"mug":{"type":"box",
 "box":{"top":0.25,"bottom":0.82,"left":0.44,"right":0.84},"confidence":0.92}},
 "usage":{"input_tokens":290,"output_tokens":21}}`

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type capturedPart struct {
	contentType string
	filename    string
	data        []byte
}

func TestSystemOneImages_multipartContract(t *testing.T) {
	photo := Image{Ref: "a3f1c2e8-door", Data: jpegBytes(t, solid(300, 400, color.White))}
	thumb := Image{Ref: "0b6c51aa-thumb", Data: pngBytes(t, solid(280, 280, color.Black))}
	badge := Image{Ref: "9a21c7d3-badge", Data: jpegBytes(t, solid(320, 320, color.Gray{0x80}))}

	var header, contentType string
	parts := map[string]capturedPart{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		header, contentType = r.Header.Get(DangerZoneImagesHeader), r.Header.Get("Content-Type")
		_, params, _ := mime.ParseMediaType(contentType)
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(p)
			parts[p.FormName()] = capturedPart{p.Header.Get("Content-Type"), p.FileName(), data}
		}
		_, _ = w.Write([]byte(okBody))
	})

	_, err := c.SystemOneImages(context.Background(),
		map[string]any{"door": photo}, // Image marshals to its Ref
		map[string]Question{
			"badge":   Noul("Is the person in `door` the person in " + badge.Ref + "?"),
			"gesture": Choice("Which gesture?", map[string]any{"thumbs_up": thumb, "none": "no hand"}),
		},
		photo, thumb, badge)
	if err != nil {
		t.Fatalf("SystemOneImages: %v", err)
	}

	if header != "true" || !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Errorf("header=%q content-type=%q", header, contentType)
	}
	if got := parts["request"].contentType; got != "application/json" {
		t.Errorf("request part content-type = %q", got)
	}
	var body struct {
		Model  string                       `json:"model"`
		State  map[string]string            `json:"state"`
		Assets map[string]map[string]string `json:"assets"`
	}
	if err := json.Unmarshal(parts["request"].data, &body); err != nil {
		t.Fatalf("request part: %v", err)
	}
	if body.State["door"] != photo.Ref || body.Model != "jev-1.13.0" {
		t.Errorf("body = %+v", body)
	}
	for n, img := range []Image{photo, thumb, badge} {
		a := body.Assets[img.Ref]
		if a["type"] != "image" || a["part"] != partName(n) {
			t.Errorf("asset %s = %v", img.Ref, a)
		}
		p := parts[partName(n)]
		if !bytes.Equal(p.data, img.Data) || p.filename == "" {
			t.Errorf("part %s: %d bytes, filename %q", partName(n), len(p.data), p.filename)
		}
	}
	if parts[partName(0)].contentType != "image/jpeg" || parts[partName(1)].contentType != "image/png" {
		t.Errorf("part content types: %q %q", parts[partName(0)].contentType, parts[partName(1)].contentType)
	}
	if len(parts) != 4 {
		t.Errorf("parts = %d, want 4", len(parts))
	}
}

func TestSystemOne_headerOnlyForImagesOrBox(t *testing.T) {
	for name, tc := range map[string]struct {
		questions map[string]Question
		want      string
	}{
		"text": {questions(), ""},
		"box":  {map[string]Question{"mug": Box("the mug in `photo`")}, "true"},
	} {
		t.Run(name, func(t *testing.T) {
			var header, contentType string
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				header, contentType = r.Header.Get(DangerZoneImagesHeader), r.Header.Get("Content-Type")
				_, _ = w.Write([]byte(okBody))
			})
			if _, err := c.SystemOne(context.Background(), nil, tc.questions); err != nil {
				t.Fatal(err)
			}
			if header != tc.want || contentType != "application/json" {
				t.Errorf("header=%q content-type=%q", header, contentType)
			}
		})
	}
}

func TestSystemOneImages_rejectsBeforeSending(t *testing.T) {
	ok := jpegBytes(t, solid(300, 300, color.White))
	padded := func(n int) []byte { return append(bytes.Clone(ok), make([]byte, n)...) }
	longRef := strings.Repeat("r", maxRefBytes+1)

	manyRefs := make([]Image, 0, 70)
	manyState := map[string]any{}
	for n := range 70 {
		img := Image{Ref: strings.Repeat(string(rune('A'+n%26)), 1000) + partName(n), Data: ok}
		manyRefs = append(manyRefs, img)
		manyState[partName(n)] = img
	}

	cases := map[string]struct {
		state  any
		images []Image
		want   string
	}{
		"unreferenced":      {map[string]any{"photo": "nothing here"}, []Image{{Ref: "ref-1", Data: ok}}, "not referenced"},
		"ref only in a key": {map[string]any{"ref-1": "x"}, []Image{{Ref: "ref-1", Data: ok}}, "not referenced"},
		"empty ref":         {map[string]any{"p": "x"}, []Image{{Ref: " ", Data: ok}}, "empty reference"},
		"ref too long":      {map[string]any{"p": longRef}, []Image{{Ref: longRef, Data: ok}}, "over 1024 bytes"},
		"refs too long":     {manyState, manyRefs, "over 64 KiB"},
		"duplicate ref": {map[string]any{"p": "ref-1"},
			[]Image{{Ref: "ref-1", Data: ok}, {Ref: "ref-1", Data: ok}}, "duplicate reference"},
		"file over 16 MiB": {map[string]any{"p": "ref-1"}, []Image{{Ref: "ref-1", Data: padded(maxImageBytes)}}, "over 16 MiB"},
		"not an image":     {map[string]any{"p": "ref-1"}, []Image{{Ref: "ref-1", Data: []byte("GIF89a")}}, "not a readable PNG or JPEG"},
		"side under 280": {map[string]any{"p": "ref-1"},
			[]Image{{Ref: "ref-1", Data: jpegBytes(t, solid(279, 600, color.White))}}, "279x600"},
		"height under 280": {map[string]any{"p": "ref-1"},
			[]Image{{Ref: "ref-1", Data: jpegBytes(t, solid(600, 279, color.White))}}, "600x279"},
		"side over 1568": {map[string]any{"p": "ref-1"},
			[]Image{{Ref: "ref-1", Data: pngBytes(t, solid(1569, 300, color.White))}}, "1569x300"},
		"height over 1568": {map[string]any{"p": "ref-1"},
			[]Image{{Ref: "ref-1", Data: pngBytes(t, solid(300, 1569, color.White))}}, "300x1569"},
		"request over 32 MiB": {map[string]any{"p": "ref-1 ref-2 ref-3"}, []Image{
			{Ref: "ref-1", Data: padded(11 << 20)}, {Ref: "ref-2", Data: padded(11 << 20)}, {Ref: "ref-3", Data: padded(11 << 20)},
		}, "over 32 MiB"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
			_, err := c.SystemOneImages(context.Background(), tc.state, map[string]Question{"q": Noul("ok?")}, tc.images...)
			if !errors.Is(err, ErrInvalidImage) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if calls.Load() != 0 {
				t.Errorf("sent %d requests", calls.Load())
			}
		})
	}
}

func TestSystemOneImages_boundsAreInclusive(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(okBody)) })
	for _, size := range [][2]int{{280, 280}, {1568, 1568}, {280, 1568}} {
		img := Image{Ref: "ref-1", Data: pngBytes(t, solid(size[0], size[1], color.White))}
		if _, err := c.SystemOneImages(context.Background(), map[string]any{"p": img}, questions(), img); err != nil {
			t.Errorf("%v: %v", size, err)
		}
	}
}

func TestSystemOne_decodesBox(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(boxBody)) })
	res, err := c.SystemOne(context.Background(), nil, map[string]Question{"mug": Box("the mug")})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Answers["mug"]
	want := Edges{Top: 0.25, Bottom: 0.82, Left: 0.44, Right: 0.84}
	if a.Type != QuestionBox || a.Box != want || a.Confidence != 0.92 || res.Model != ModelJev114 {
		t.Errorf("answer = %+v model %s", a, res.Model)
	}
	if got := a.Box.Pixels(1000, 500); got != image.Rect(440, 125, 840, 410) {
		t.Errorf("Pixels = %v", got)
	}
}

func TestEdgesPixels_clamps(t *testing.T) {
	got := Edges{Top: -0.1, Bottom: 1.2, Left: 0, Right: 1}.Pixels(300, 200)
	if got != image.Rect(0, 0, 300, 200) {
		t.Errorf("Pixels = %v", got)
	}
}

func TestBoxQuestion_hasNoCriteria(t *testing.T) {
	js, _ := json.Marshal(Box("the mug in `photo`"))
	if string(js) != `{"type":"box","instructions":"the mug in `+"`photo`"+`"}` {
		t.Errorf("got %s", js)
	}
}

// OpenRouter rejects multipart ("invalid content-type: multipart/form-data",
// live 2026-11-09), so it gets JSON with base64 assets.
func TestSystemOneImages_openRouterInlinesBase64(t *testing.T) {
	photo := Image{Ref: "6e1d0b7c-square", Data: jpegBytes(t, solid(300, 400, color.White))}
	diagram := Image{Ref: "77b0d4a1-diagram", Data: pngBytes(t, solid(280, 280, color.Black))}
	var header, contentType string
	var body struct {
		Assets map[string]map[string]string `json:"assets"`
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		header, contentType = r.Header.Get(DangerZoneImagesHeader), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(boxBody))
	})
	c.providerName = "openrouter"

	_, err := c.SystemOneImages(context.Background(), map[string]any{"photo": photo, "ref": diagram},
		map[string]Question{"mug": Box("the mug in `photo`")}, photo, diagram)
	if err != nil {
		t.Fatal(err)
	}
	if header != "true" || contentType != "application/json" {
		t.Errorf("header=%q content-type=%q", header, contentType)
	}
	for img, format := range map[*Image]string{&photo: "jpeg", &diagram: "png"} {
		a := body.Assets[img.Ref]
		data, _ := base64.StdEncoding.DecodeString(a["data_base64"])
		if a["type"] != "image" || a["format"] != format || a["part"] != "" || !bytes.Equal(data, img.Data) {
			t.Errorf("asset %s: type=%q format=%q part=%q %d bytes", img.Ref, a["type"], a["format"], a["part"], len(data))
		}
	}
}
