package color

import (
	"fmt"
	"os"
)

var enabled = true

// Init also honors the NO_COLOR convention: https://no-color.org
func Init(noColor bool) {
	if noColor {
		enabled = false
		return
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		enabled = false
		return
	}
	enabled = true
}

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	cyan   = "\x1b[36m"
	gray   = "\x1b[90m"
)

func wrap(code, s string) string {
	if !enabled {
		return s
	}
	return code + s + reset
}

func Bold(s string) string   { return wrap(bold, s) }
func Red(s string) string    { return wrap(red, s) }
func Green(s string) string  { return wrap(green, s) }
func Yellow(s string) string { return wrap(yellow, s) }
func Blue(s string) string   { return wrap(blue, s) }
func Cyan(s string) string   { return wrap(cyan, s) }
func Gray(s string) string   { return wrap(gray, s) }

func Sprintf(code, format string, args ...any) string {
	return wrap(code, fmt.Sprintf(format, args...))
}
