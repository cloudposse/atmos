package logger

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/fatih/color"
)

const (
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

type Logger struct {
	LogLevel string
	LogFile  string
}

func (l *Logger) Error(err error) {
	if err != nil {
		c := theme.Colors.Error
		_, printErr := c.Fprintln(color.Error, err.Error()+"\n")
		if printErr != nil {
			theme.Colors.Error.Println("Error logging the error:")
			theme.Colors.Error.Printf("%s\n", printErr)
			theme.Colors.Error.Println("Original error:")
			theme.Colors.Error.Printf("%s\n", err)
		}
	}
}

func (l *Logger) Trace(message string) {
	if l.LogLevel == LogLevelTrace {
		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Debug(message string) {
	if l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug {

		l.log(theme.Colors.Info, message)
	}
}

func (l *Logger) Info(message string) {
	if l.LogLevel == "" ||
		l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug ||
		l.LogLevel == LogLevelInfo {

		l.log(theme.Colors.Default, message)
	}
}

func (l *Logger) Warning(message string) {
	if l.LogLevel == LogLevelTrace ||
		l.LogLevel == LogLevelDebug ||
		l.LogLevel == LogLevelInfo ||
		l.LogLevel == LogLevelWarning {

		l.log(theme.Colors.Warning, message)
	}
}

func (l *Logger) log(logColor *color.Color, message string) {
	if l.LogFile != "" {
		if l.LogFile == "/dev/stdout" {
			_, err := logColor.Fprintln(os.Stdout, message)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		} else if l.LogFile == "/dev/stderr" {
			_, err := logColor.Fprintln(os.Stderr, message)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		} else {
			f, err := os.OpenFile(l.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
				return
			}

			defer func(f *os.File) {
				err = f.Close()
				if err != nil {
					theme.Colors.Error.Printf("%s\n", err)
				}
			}(f)

			_, err = f.Write([]byte(fmt.Sprintf("%s\n", message)))
			if err != nil {
				theme.Colors.Error.Printf("%s\n", err)
			}
		}
	} else {
		_, err := logColor.Fprintln(os.Stdout, message)
		if err != nil {
			theme.Colors.Error.Printf("%s\n", err)
		}
	}
}
