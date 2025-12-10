package utils

import (
	"image"
	"image/draw"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/avif"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

func ConvertAvatar(file multipart.File, path, filename string) error {
	img, _, err := image.Decode(file)
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
