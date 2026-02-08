package logging

import (
	"fmt"
	"os"
	"time"
)

type Logger struct {
	verbose bool
}

type Config struct {
	Level   string
	Verbose bool
	Quiet   bool
}

func NewLogger(cfg Config) *Logger {
	return &Logger{verbose: cfg.Verbose}
}

func (l *Logger) Info(format string, args ...interface{}) {
	fmt.Printf("● "+format+"\n", args...)
}

func (l *Logger) InfoWithTime(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] "+format+"\n", append([]interface{}{timestamp}, args...)...)
}

func (l *Logger) Success(format string, args ...interface{}) {
	fmt.Printf("\033[32m✓ "+format+"\033[0m\n", args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	fmt.Printf("\033[33m⚠ "+format+"\033[0m\n", args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\033[31m✗ "+format+"\033[0m\n", args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.verbose {
		fmt.Printf("  "+format+"\n", args...)
	}
}
