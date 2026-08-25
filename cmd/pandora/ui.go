package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// UI contains formatting and prompt helper utilities
type UI struct {
	Out io.Writer
	In  io.Reader
}

// NewUI creates a UI instance
func NewUI(in io.Reader, out io.Writer) *UI {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	return &UI{In: in, Out: out}
}

// Banner prints the Pandora's Veil header
func (u *UI) Banner() {
	fmt.Fprintf(u.Out, "%s%s==================================================%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Fprintf(u.Out, "%s%s   PANDORA'S VEIL — ZERO-KNOWLEDGE SECRET RELAY   %s\n", ColorBold, ColorWhite, ColorReset)
	fmt.Fprintf(u.Out, "%s%s   Device-Bound Cryptographic Delivery System     %s\n", ColorDim, ColorCyan, ColorReset)
	fmt.Fprintf(u.Out, "%s%s==================================================%s\n\n", ColorBold, ColorCyan, ColorReset)
}

// Success prints a green success message
func (u *UI) Success(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(u.Out, "%s%s[✓]%s %s\n", ColorBold, ColorGreen, ColorReset, msg)
}

// Info prints a cyan info message
func (u *UI) Info(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(u.Out, "%s%s[i]%s %s\n", ColorBold, ColorCyan, ColorReset, msg)
}

// Warn prints a yellow warning message
func (u *UI) Warn(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(u.Out, "%s%s[!]%s %s\n", ColorBold, ColorYellow, ColorReset, msg)
}

// Error prints a red error message
func (u *UI) Error(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(u.Out, "%s%s[✗]%s %s\n", ColorBold, ColorRed, ColorReset, msg)
}

// PromptConfirm asks a mandatory y/N confirmation prompt
// Returns true ONLY if user inputs 'y' or 'yes' (case-insensitive)
func (u *UI) PromptConfirm(promptText string) bool {
	fmt.Fprintf(u.Out, "%s%s%s [y/N]: %s", ColorBold, ColorYellow, promptText, ColorReset)
	scanner := bufio.NewScanner(u.In)
	if scanner.Scan() {
		text := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return text == "y" || text == "yes"
	}
	return false
}

func isBoolFlag(f string) bool {
	switch f {
	case "-burn", "--burn", "-force", "--force", "-h", "-help", "--help", "-v", "--version", "-version":
		return true
	default:
		return false
	}
}

// normalizeArgs places all flags before positional arguments so flag.FlagSet parses correctly
func normalizeArgs(args []string) []string {
	var flags []string
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && !isBoolFlag(arg) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, arg)
		}
	}
	return append(flags, pos...)
}

