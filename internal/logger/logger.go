// Package logger provides styled, ANSI-colored logging functions for
// professional terminal output. Zero external dependencies.
package logger

import (
	"fmt"
	"log"
	"time"
)

// ANSI color escape codes.
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	cyan    = "\033[36m"
	blue    = "\033[34m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	red     = "\033[31m"
	magenta = "\033[35m"
	white   = "\033[37m"
)

func init() {
	// Disable the default log timestamp — we print our own.
	log.SetFlags(0)
}

// timestamp returns a dim HH:MM:SS string for log lines.
func timestamp() string {
	return fmt.Sprintf("%s%s%s", dim, time.Now().Format("15:04:05"), reset)
}

// Info logs an informational message with a cyan [INFO] prefix.
func Info(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[INFO]%s  %s", timestamp(), bold, cyan, reset, text)
}

// System logs a system-level message with a blue [SYS] prefix.
func System(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[SYS]%s   %s", timestamp(), bold, blue, reset, text)
}

// Success logs a success message with a green [OK] prefix.
func Success(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[OK]%s    %s", timestamp(), bold, green, reset, text)
}

// Warning logs a warning message with a yellow [WARN] prefix.
func Warning(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[WARN]%s  %s", timestamp(), bold, yellow, reset, text)
}

// Error logs an error message with a red [ERR] prefix.
func Error(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[ERR]%s   %s", timestamp(), bold, red, reset, text)
}

// Memory logs a memory/scratchpad message with a magenta [MEM] prefix.
func Memory(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	log.Printf("%s %s%s[MEM]%s   %s", timestamp(), bold, magenta, reset, text)
}
