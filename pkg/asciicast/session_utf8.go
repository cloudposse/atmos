package asciicast

import "unicode/utf8"

// completeUTF8Prefix retains an incomplete trailing rune until the next PTY read.
// JSON replaces invalid UTF-8, so encoding split rune bytes as separate events
// permanently corrupts progress bars and other multibyte terminal characters.
// Invalid bytes that cannot become a valid rune are emitted without waiting.
func completeUTF8Prefix(data []byte) int {
	start := len(data) - 1
	for start >= 0 && !utf8.RuneStart(data[start]) {
		start--
	}
	if start >= 0 && !utf8.FullRune(data[start:]) {
		return start
	}
	return len(data)
}
