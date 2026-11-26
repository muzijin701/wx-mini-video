package logo

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"
	"sync"
)

// 个人紫色狸花猫图标（全身透明底主标识浅色 / 深色版本），通过 go:embed 编译进二进制。
//
//go:embed assets/logo-light.png
var logoLightPNG []byte

//go:embed assets/logo-dark.png
var logoDarkPNG []byte

type cachedLogo struct {
	once sync.Once
	img  *image.RGBA
	err  error
}

var (
	lightLogo cachedLogo
	darkLogo  cachedLogo
)

// decodeLogo 解码内置 512x512 RGBA 个人品牌图标（按浅色/深色主题选择变体）。
func decodeLogo(dark bool) (*image.RGBA, error) {
	entry := &lightLogo
	raw := logoLightPNG
	if dark {
		entry = &darkLogo
		raw = logoDarkPNG
	}
	entry.once.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			entry.err = err
			return
		}
		if rgba, ok := decoded.(*image.RGBA); ok {
			entry.img = rgba
			return
		}
		bounds := decoded.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
		entry.img = rgba
	})
	return entry.img, entry.err
}

// LogoSize 报告内置图标尺寸（预期浅色、深色均为 512x512）。
func LogoSize() (width, height int, err error) {
	img, err := decodeLogo(true)
	if err != nil {
		return 0, 0, err
	}
	if _, err := decodeLogo(false); err != nil {
		return 0, 0, err
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}
