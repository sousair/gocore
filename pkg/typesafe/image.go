package typesafe

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"mime/multipart"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// DangerZoneImagesHeader must be "true" on every request carrying an image or a
// Box question; the API answers 400 without it.
const DangerZoneImagesHeader = "TypeSafe-Danger-Zone-Images"

const (
	maxImageBytes    = 16 << 20
	maxRequestBytes  = 32 << 20
	minImageSide     = 280
	maxImageSide     = 1568
	maxRefBytes      = 1024
	maxTotalRefBytes = 64 << 10
)

var ErrInvalidImage = errors.New("typesafe: invalid image")

// Image is a PNG or JPEG still sent as an asset. The API swaps Ref for the
// image wherever Ref appears in a string value of state, instructions or
// criteria, mid-sentence included. Image marshals to its Ref, so it can be
// placed directly in state or criteria values.
type Image struct {
	Ref  string
	Data []byte
}

// NewImage wraps data under a fresh random reference.
func NewImage(data []byte) Image {
	return Image{Ref: uuid.NewString(), Data: data}
}

func (i Image) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.Ref)
}

// asset names a multipart file part (Part) or carries the image inline
// (DataBase64 with Format).
type asset struct {
	Type       string `json:"type"`
	Part       string `json:"part,omitempty"`
	Format     string `json:"format,omitempty"`
	DataBase64 string `json:"data_base64,omitempty"`
}

// validate checks the documented per-image limits and returns the format
// ("jpeg" or "png").
// The total-pixel bounds (280² to 1568²) follow from the per-side bounds.
func (i Image) validate() (string, error) {
	ref := strings.TrimSpace(i.Ref)
	switch {
	case ref == "":
		return "", fmt.Errorf("%w: empty reference", ErrInvalidImage)
	case len(i.Ref) > maxRefBytes:
		return "", fmt.Errorf("%w: reference %.32q… is over %d bytes", ErrInvalidImage, i.Ref, maxRefBytes)
	case len(i.Data) > maxImageBytes:
		return "", fmt.Errorf("%w: %q is %d bytes, over 16 MiB", ErrInvalidImage, i.Ref, len(i.Data))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(i.Data))
	if err != nil {
		return "", fmt.Errorf("%w: %q is not a readable PNG or JPEG: %w", ErrInvalidImage, i.Ref, err)
	}
	if cfg.Width < minImageSide || cfg.Height < minImageSide || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return "", fmt.Errorf("%w: %q is %dx%d, each side must be %d-%d px (see PrepareImage)",
			ErrInvalidImage, i.Ref, cfg.Width, cfg.Height, minImageSide, maxImageSide)
	}
	return format, nil
}

// imageBody checks every image is valid and referenced, then encodes req
// either as JSON with base64 assets (inline) or as the "request" JSON part
// plus one file part per image. OpenRouter accepts only the inline form.
func imageBody(req request, images []Image, inline bool) ([]byte, string, error) {
	formats := make([]string, len(images))
	req.Assets = make(map[string]asset, len(images))
	for n, img := range images {
		format, err := img.validate()
		if err != nil {
			return nil, "", err
		}
		if _, dup := req.Assets[img.Ref]; dup {
			return nil, "", fmt.Errorf("%w: duplicate reference %q", ErrInvalidImage, img.Ref)
		}
		formats[n] = format
		a := asset{Type: "image", Part: partName(n)}
		if inline {
			a = asset{Type: "image", Format: format, DataBase64: base64.StdEncoding.EncodeToString(img.Data)}
		}
		req.Assets[img.Ref] = a
	}
	if err := checkReferenced(req, images); err != nil {
		return nil, "", err
	}
	js, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("typesafe: marshal: %w", err)
	}
	if inline {
		return checkSize(js, "application/json")
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := writePart(w, "request", "", "application/json", js); err != nil {
		return nil, "", err
	}
	for n, img := range images {
		if err := writePart(w, partName(n), partName(n), "image/"+formats[n], img.Data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("typesafe: multipart: %w", err)
	}
	return checkSize(buf.Bytes(), w.FormDataContentType())
}

func checkSize(body []byte, contentType string) ([]byte, string, error) {
	if len(body) > maxRequestBytes {
		return nil, "", fmt.Errorf("%w: request is %d bytes, over 32 MiB", ErrInvalidImage, len(body))
	}
	return body, contentType, nil
}

func partName(n int) string { return "image-" + strconv.Itoa(n) }

func writePart(w *multipart.Writer, name, filename, contentType string, data []byte) error {
	h := textproto.MIMEHeader{}
	disposition := `form-data; name="` + name + `"`
	if filename != "" {
		disposition += `; filename="` + filename + `"`
	}
	h.Set("Content-Disposition", disposition)
	h.Set("Content-Type", contentType)
	p, err := w.CreatePart(h)
	if err == nil {
		_, err = p.Write(data)
	}
	if err != nil {
		return fmt.Errorf("typesafe: multipart: %w", err)
	}
	return nil
}

// checkReferenced fails when an image's Ref appears in no string value of the
// state or questions: the API rejects unreferenced assets with a 400. Object
// keys are skipped because the API does not replace references in keys.
func checkReferenced(req request, images []Image) error {
	js, err := json.Marshal(struct {
		State     any                 `json:"state"`
		Questions map[string]Question `json:"questions"`
	}{req.State, req.Questions})
	if err != nil {
		return fmt.Errorf("typesafe: marshal: %w", err)
	}
	var tree any
	if err := json.Unmarshal(js, &tree); err != nil {
		return fmt.Errorf("typesafe: marshal: %w", err)
	}
	var values []string
	collectStrings(tree, &values)

	total := 0
	for _, img := range images {
		total += len(img.Ref)
		if !referenced(img.Ref, values) {
			return fmt.Errorf("%w: %q is not referenced in state, instructions or criteria", ErrInvalidImage, img.Ref)
		}
	}
	if total > maxTotalRefBytes {
		return fmt.Errorf("%w: references total %d bytes, over 64 KiB", ErrInvalidImage, total)
	}
	return nil
}

func referenced(ref string, values []string) bool {
	for _, v := range values {
		if strings.Contains(v, ref) {
			return true
		}
	}
	return false
}

func collectStrings(v any, out *[]string) {
	switch v := v.(type) {
	case string:
		*out = append(*out, v)
	case []any:
		for _, e := range v {
			collectStrings(e, out)
		}
	case map[string]any:
		for _, e := range v {
			collectStrings(e, out)
		}
	}
}
