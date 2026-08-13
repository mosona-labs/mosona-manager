package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
	"path/filepath"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/avif"
	_ "golang.org/x/image/webp"
)

const (
	MaxAvatarBytes        = 5 << 20
	MaxAvatarRequestBytes = MaxAvatarBytes + 1<<20
	MaxAvatarWidth        = 4096
	MaxAvatarHeight       = 4096
	MaxAvatarPixels       = 12_000_000
)

var allowedAvatarFormats = map[string]struct{}{
	"jpeg": {},
	"png":  {},
	"gif":  {},
	"webp": {},
}

func ConvertAvatar(file io.Reader, path, filename string) error {
	data, err := io.ReadAll(io.LimitReader(file, MaxAvatarBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxAvatarBytes {
		return fmt.Errorf("avatar image exceeds %d bytes", MaxAvatarBytes)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if _, ok := allowedAvatarFormats[format]; !ok {
		return fmt.Errorf("unsupported avatar image format: %s", format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxAvatarWidth || cfg.Height > MaxAvatarHeight || cfg.Width*cfg.Height > MaxAvatarPixels {
		return fmt.Errorf("avatar image dimensions exceed limit")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	size := width
	if height < width {
		size = height
	}

	x := (width - size) / 2
	y := (height - size) / 2
	cropped := imaging.Crop(img, image.Rect(x, y, x+size, y+size))

	resized := imaging.Resize(cropped, 256, 256, imaging.Lanczos)

	rgba := image.NewRGBA(resized.Bounds())
	draw.Draw(rgba, resized.Bounds(), resized, image.Point{}, draw.Src)

	out, err := os.Create(
		filepath.Join(path, filename+".avif"),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	return avif.Encode(out, rgba, avif.Options{
		Quality: 80,
	})
}
