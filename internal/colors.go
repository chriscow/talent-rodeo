package internal

import "github.com/fatih/color"

var (
	Black   = color.New(color.FgBlack).SprintFunc()
	Red     = color.New(color.FgRed).SprintFunc()
	Green   = color.New(color.FgGreen).SprintFunc()
	Yellow  = color.New(color.FgYellow).SprintFunc()
	Blue    = color.New(color.FgBlue).SprintFunc()
	Magenta = color.New(color.FgMagenta).SprintFunc()
	Cyan    = color.New(color.FgCyan).SprintFunc()
	Gray    = color.New(color.FgWhite).SprintFunc()

	HiBlack   = color.New(color.FgHiBlack).SprintFunc()
	HiRed     = color.New(color.FgHiRed).SprintFunc()
	HiGreen   = color.New(color.FgHiGreen).SprintFunc()
	HiYellow  = color.New(color.FgHiYellow).SprintFunc()
	HiBlue    = color.New(color.FgHiBlue).SprintFunc()
	HiMagenta = color.New(color.FgHiMagenta).SprintFunc()
	HiCyan    = color.New(color.FgHiCyan).SprintFunc()
	HiWhite   = color.New(color.FgHiWhite).SprintFunc()

	BGRed     = color.New(color.BgRed, color.FgBlack).SprintFunc()
	BGGreen   = color.New(color.BgGreen, color.FgBlack).SprintFunc()
	BGYellow  = color.New(color.BgYellow, color.FgBlack).SprintFunc()
	BGBlue    = color.New(color.BgBlue, color.FgBlack).SprintFunc()
	BGMagenta = color.New(color.BgMagenta, color.FgBlack).SprintFunc()

	HiBGRed     = color.New(color.BgHiRed, color.FgBlack).SprintFunc()
	HiBGGreen   = color.New(color.BgHiGreen, color.FgBlack).SprintFunc()
	HiBGYellow  = color.New(color.BgHiYellow, color.FgBlack).SprintFunc()
	HiBGBlue    = color.New(color.BgHiBlue, color.FgBlack).SprintFunc()
	HiBGMagenta = color.New(color.BgHiMagenta, color.FgBlack).SprintFunc()
	HiBGCyan    = color.New(color.BgHiCyan, color.FgBlack).SprintFunc()
	HiBGWhite   = color.New(color.BgHiWhite, color.FgBlack).SprintFunc()
)
