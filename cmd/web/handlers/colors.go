package handlers

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

type ColorPalette struct {
	Theme  string   `json:"theme"`
	Colors []string `json:"colors"`
}

func containsColorPaletteRequest(message string) bool {
	// Add more sophisticated detection logic here
	return contains(message, []string{
		"color palette",
		"color scheme",
		"colors",
		"palette",
	})
}

func generateColorPalette(theme string) ColorPalette {
	// Pre-defined color sets - expand this for production
	palettes := map[string][]string{
		"bright and modern": {
			"#FF6B6B",
			"#4ECDC4",
			"#45B7D1",
			"#96CEB4",
			"#FFEEAD",
		},
		"warm earth": {
			"#D4A373",
			"#CCD5AE",
			"#E9EDC9",
			"#FEFAE0",
			"#FAEDCD",
		},
	}

	if colors, ok := palettes[theme]; ok {
		return ColorPalette{
			Theme:  theme,
			Colors: colors,
		}
	}

	// Fallback to random bright colors
	colors := make([]string, 5)
	for i := range colors {
		h := rand.Float64() * 360
		s := 0.7 + rand.Float64()*0.3 // 70-100% saturation
		l := 0.4 + rand.Float64()*0.2 // 40-60% lightness
		colors[i] = hslToHex(h, s, l)
	}

	return ColorPalette{
		Theme:  theme,
		Colors: colors,
	}
}

func contains(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(strings.ToLower(s), strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// hslToHex converts HSL values to a HEX color code.
func hslToHex(h, s, l float64) string {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case 0 <= h && h < 60:
		r, g, b = c, x, 0
	case 60 <= h && h < 120:
		r, g, b = x, c, 0
	case 120 <= h && h < 180:
		r, g, b = 0, c, x
	case 180 <= h && h < 240:
		r, g, b = 0, x, c
	case 240 <= h && h < 300:
		r, g, b = x, 0, c
	case 300 <= h && h < 360:
		r, g, b = c, 0, x
	}

	r = (r + m) * 255
	g = (g + m) * 255
	b = (b + m) * 255

	return fmt.Sprintf("#%02X%02X%02X", int(r), int(g), int(b))
}
