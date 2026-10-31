package compat

import (
	"bytes"
	"io"
)

func bytesReader(s string) io.Reader { return bytes.NewReader([]byte(s)) }
