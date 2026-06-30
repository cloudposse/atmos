package marketplace

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

var errIOContextNotInitialized = errors.New("I/O context is not initialized")

func writeDataf(format string, args ...any) error {
	ctx := iolib.GetContext()
	if ctx == nil {
		return errIOContextNotInitialized
	}
	return ctx.Write(iolib.DataStream, fmt.Sprintf(format, args...))
}

func readInputLine() (string, error) {
	ctx := iolib.GetContext()
	if ctx == nil {
		return "", errIOContextNotInitialized
	}
	reader := bufio.NewReader(ctx.Input())
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
