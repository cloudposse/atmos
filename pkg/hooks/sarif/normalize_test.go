package sarif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathFromSARIFURI_WindowsDrivePathIsNotURLScheme(t *testing.T) {
	for _, uri := range []string{
		`C:/Users/runneradmin/AppData/Local/Temp/repo/main.tf`,
		`C:\Users\runneradmin\AppData\Local\Temp\repo\main.tf`,
	} {
		t.Run(uri, func(t *testing.T) {
			got, ok := pathFromSARIFURI(uri)
			assert.True(t, ok)
			assert.Equal(t, uri, got)
		})
	}
}

func TestPathFromSARIFURI_ExternalURLIsRejected(t *testing.T) {
	_, ok := pathFromSARIFURI("https://example.com/main.tf")
	assert.False(t, ok)
}
