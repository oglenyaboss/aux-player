package art

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestDecode_SupportsPNGAndJPEG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 90, G: 80, B: 70, A: 255})

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	if decoded, err := Decode(&pngBuf); err != nil || decoded.Bounds().Dx() != 1 {
		t.Fatalf("Decode(PNG) = %v, %v; want image, nil", decoded, err)
	}

	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if decoded, err := Decode(&jpegBuf); err != nil || decoded.Bounds().Dy() != 1 {
		t.Fatalf("Decode(JPEG) = %v, %v; want image, nil", decoded, err)
	}
}

func TestDecode_RejectsUnsupportedBytes(t *testing.T) {
	if _, err := Decode(strings.NewReader("not an image")); err == nil {
		t.Fatal("Decode(invalid) error = nil, want error")
	}
}

func TestFetchAndDecode_SupportsPNGDataURL(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 12, G: 34, B: 56, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	decoded, err := FetchAndDecode(t.Context(), nil, url, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 1 || decoded.Bounds().Dy() != 1 {
		t.Fatalf("decoded bounds = %v, want 1x1", decoded.Bounds())
	}
}

func TestDecodeBounded_RejectsOverByteCap(t *testing.T) {
	if _, err := decodeBounded(strings.NewReader("abcdef"), 5); err == nil || !strings.Contains(err.Error(), "exceeds max bytes") {
		t.Fatalf("decodeBounded(over cap) error = %v, want exceeds max bytes", err)
	}
}

func TestDecodeBounded_RejectsOversizedDimensions(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, MaxArtworkWidth+1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeBounded(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err == nil || !strings.Contains(err.Error(), "exceed max") {
		t.Fatalf("decodeBounded(oversized) error = %v, want exceed max", err)
	}
}
