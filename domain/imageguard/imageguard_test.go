package imageguard

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodedJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		img.Set(x, x%height, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var limits = Limits{MaxBytes: 1 << 20, MinWidth: 32, MinHeight: 16, MaxWidth: 2048, MaxHeight: 2048}

func TestCheckAcceptsADecodableJPEGInsideTheLimits(t *testing.T) {
	size, err := Check(encodedJPEG(t, 400, 300), limits)
	if err != nil || size != (Size{Width: 400, Height: 300}) {
		t.Fatalf("Check() = %+v, %v", size, err)
	}
}

func TestCheckRefusesWhatIsNotAnHonestJPEG(t *testing.T) {
	good := encodedJPEG(t, 400, 300)
	cases := map[string][]byte{
		"empty":        nil,
		"png":          encodedPNG(t, 400, 300),
		"html":         []byte("<html><script>alert(1)</script></html>"),
		"truncated":    good[:len(good)/2],
		"too small":    encodedJPEG(t, 20, 300),
		"too short":    encodedJPEG(t, 400, 10),
		"too wide":     encodedJPEG(t, 2049, 20),
		"too tall":     encodedJPEG(t, 40, 2049),
		"too heavy":    append(append([]byte{}, good...), make([]byte, 1<<20)...),
		"magic only":   {0xFF, 0xD8, 0xFF, 0xE0},
		"gif disguise": []byte("GIF89a\x01\x00\x01\x00"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Check(data, limits); !errors.Is(err, ErrRefused) {
				t.Fatalf("Check() = %v, want ErrRefused", err)
			}
		})
	}
}

func TestCleanReencodesSoNothingButPixelsSurvives(t *testing.T) {
	original := encodedJPEG(t, 400, 300)
	marker := []byte("vz-hidden-payload")
	tampered := append(append(append([]byte{}, original[:2]...), append([]byte{0xFF, 0xFE, 0x00, byte(len(marker) + 2)}, marker...)...), original[2:]...)
	if _, err := Check(tampered, limits); err != nil {
		t.Fatalf("the tampered file still decodes: %v", err)
	}
	clean, size, err := Clean(tampered, limits)
	if err != nil || size != (Size{Width: 400, Height: 300}) {
		t.Fatalf("Clean() = %+v, %v", size, err)
	}
	if bytes.Contains(clean, marker) {
		t.Fatal("the comment segment survived the re-encode")
	}
	if _, err := Check(clean, limits); err != nil {
		t.Fatalf("the clean file is a valid JPEG: %v", err)
	}
}

func TestJPEGDataURLDecodesOnlyAJPEGDataURL(t *testing.T) {
	raw := encodedJPEG(t, 64, 32)
	data, err := JPEGDataURL("data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(raw), limits)
	if err != nil || !bytes.Equal(data, raw) {
		t.Fatalf("JPEGDataURL() = %d bytes, %v", len(data), err)
	}
	for name, value := range map[string]string{
		"png prefix": "data:image/png;base64," + base64.StdEncoding.EncodeToString(encodedPNG(t, 64, 32)),
		"no prefix":  base64.StdEncoding.EncodeToString(raw),
		"bad base64": "data:image/jpeg;base64,***",
		"svg":        "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := JPEGDataURL(value, limits); !errors.Is(err, ErrRefused) {
				t.Fatalf("JPEGDataURL() = %v, want ErrRefused", err)
			}
		})
	}
}
