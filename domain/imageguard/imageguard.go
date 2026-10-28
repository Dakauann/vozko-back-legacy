package imageguard

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
)

const (
	JPEGDataURLPrefix = "data:image/jpeg;base64,"
	cleanQuality      = 85
)

var (
	ErrRefused = errors.New("imageguard: the image is refused")
	jpegMagic  = []byte{0xFF, 0xD8, 0xFF}
)

type Limits struct {
	MaxBytes  int
	MinWidth  int
	MinHeight int
	MaxWidth  int
	MaxHeight int
}

type Size struct {
	Width  int
	Height int
}

func refused(reason string) error {
	return fmt.Errorf("%w: %s", ErrRefused, reason)
}

func Check(data []byte, l Limits) (Size, error) {
	_, size, err := decode(data, l)
	return size, err
}

func Clean(data []byte, l Limits) ([]byte, Size, error) {
	img, size, err := decode(data, l)
	if err != nil {
		return nil, Size{}, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: cleanQuality}); err != nil {
		return nil, Size{}, refused(err.Error())
	}
	if buf.Len() > l.MaxBytes {
		return nil, Size{}, refused("the clean image is larger than allowed")
	}
	return buf.Bytes(), size, nil
}

func DataURL(value string, maxBytes int) ([]byte, error) {
	payload, ok := strings.CutPrefix(value, JPEGDataURLPrefix)
	if !ok {
		return nil, refused("not a JPEG data URL")
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > maxBytes+2 {
		return nil, refused("the image is larger than allowed")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, refused("the data URL is not base64")
	}
	return data, nil
}

func JPEGDataURL(value string, l Limits) ([]byte, error) {
	data, err := DataURL(value, l.MaxBytes)
	if err != nil {
		return nil, err
	}
	if _, err := Check(data, l); err != nil {
		return nil, err
	}
	return data, nil
}

func decode(data []byte, l Limits) (image.Image, Size, error) {
	if len(data) == 0 || len(data) > l.MaxBytes {
		return nil, Size{}, refused("the image size is out of range")
	}
	if !bytes.HasPrefix(data, jpegMagic) {
		return nil, Size{}, refused("the file is not a JPEG")
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, Size{}, refused("the JPEG header is unreadable")
	}
	size := Size{Width: config.Width, Height: config.Height}
	if size.Width < l.MinWidth || size.Height < l.MinHeight || size.Width > l.MaxWidth || size.Height > l.MaxHeight {
		return nil, Size{}, refused(fmt.Sprintf("%dx%d is out of range", size.Width, size.Height))
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, Size{}, refused("the JPEG does not decode")
	}
	return img, size, nil
}
