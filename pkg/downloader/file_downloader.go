package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v2"
)

var ErrFailedToProcessHclFile = errors.New("failed to process HCL file")

// fileDownloader handles downloading files and directories from various sources
// without exposing the underlying implementation.
type fileDownloader struct {
	clientFactory     ClientFactory
	tempPathGenerator func() string
	fileReader        func(string) ([]byte, error)
}

// NewFileDownloader initializes a FileDownloader with dependency injection.
func NewFileDownloader(factory ClientFactory) FileDownloader {
	return &fileDownloader{
		clientFactory:     factory,
		tempPathGenerator: func() string { return filepath.Join(os.TempDir(), uuid.New().String()) },
		fileReader:        os.ReadFile,
	}
}

// Fetch fetches content from a given source and saves it to the destination.
func (fd *fileDownloader) Fetch(src, dest string, mode ClientMode, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := fd.clientFactory.NewClient(ctx, src, dest, mode)
	if err != nil {
		return fmt.Errorf("failed to create download client: %w", err)
	}

	return client.Get()
}

// FetchAutoParse downloads a remote file, detects its format, and parses it.
func (fd *fileDownloader) FetchAndAutoParse(src string) (any, error) {
	filePath := fd.tempPathGenerator()

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf("failed to download file '%s': %w", src, err)
	}

	return fd.detectFormatAndParse(filePath)
}

func (fd *fileDownloader) detectFormatAndParse(filename string) (any, error) {
	var v any

	var err error

	d, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	data := string(d)
	switch {
	case filetype.IsJSON(data):
		err = json.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	case filetype.IsHCL(data):
		parser := hclparse.NewParser()
		file, diags := parser.ParseHCL(d, filename)
		if diags != nil && diags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
		}
		if file == nil {
			return nil, fmt.Errorf("%w, file: %s, file parsing returned nil", ErrFailedToProcessHclFile, filename)
		}

		// Extract all attributes from the file body
		attributes, diags := file.Body.JustAttributes()
		if diags != nil && diags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
		}

		// Map to store the parsed attribute values
		result := make(map[string]any)

		// Evaluate each attribute and store it in the result map
		for name, attr := range attributes {
			ctyValue, diags := attr.Expr.Value(nil)
			if diags != nil && diags.HasErrors() {
				return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
			}

			// Convert cty.Value to appropriate Go type
			result[name] = ctyToGo(ctyValue)
		}
		v = result
	case filetype.IsYAML(data):
		err = yaml.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	default:
		v = data
	}

	return v, nil
}

// CtyToGo converts cty.Value to Go types.
func ctyToGo(value cty.Value) any {
	switch {
	case value.Type().IsObjectType(): // Handle maps
		m := map[string]any{}
		for k, v := range value.AsValueMap() {
			m[k] = ctyToGo(v)
		}
		return m

	case value.Type().IsListType() || value.Type().IsTupleType(): // Handle lists
		var list []any
		for _, v := range value.AsValueSlice() {
			list = append(list, ctyToGo(v))
		}
		return list

	case value.Type() == cty.String: // Handle strings
		return value.AsString()

	case value.Type() == cty.Number: // Handle numbers
		if n, _ := value.AsBigFloat().Int64(); true {
			return n // Convert to int64 if possible
		}
		return value.AsBigFloat() // Otherwise, keep as float64

	case value.Type() == cty.Bool: // Handle booleans
		return value.True()

	default:
		return value // Return as-is for unsupported types
	}
}
