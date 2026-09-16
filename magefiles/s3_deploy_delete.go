//go:build mage

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (d *s3Deployer) deleteRemoved(location s3DeployLocation, deleted []string, tempDir string) error {
	for offset := 0; offset < len(deleted); offset += s3DeleteBatchSize {
		end := min(offset+s3DeleteBatchSize, len(deleted))
		objects := make([]map[string]string, 0, end-offset)
		for _, relative := range deleted[offset:end] {
			key := relative
			if location.Prefix != "" {
				key = location.Prefix + "/" + relative
			}
			objects = append(objects, map[string]string{"Key": key})
		}
		request := struct {
			Objects []map[string]string `json:"Objects"`
			Quiet   bool                `json:"Quiet"`
		}{Objects: objects, Quiet: true}
		data, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("mage: encode S3 delete request: %w", err)
		}
		requestPath := filepath.Join(tempDir, fmt.Sprintf("delete-%d.json", offset/s3DeleteBatchSize))
		if err := os.WriteFile(requestPath, data, s3FilePermissions); err != nil {
			return fmt.Errorf("mage: write S3 delete request: %w", err)
		}
		output, err := d.runAWSOutput(
			"s3api", "delete-objects", "--bucket", location.Bucket,
			"--delete", "file://"+requestPath, "--output", "json",
		)
		if err != nil {
			return err
		}
		if err := validateS3DeleteResponse(output); err != nil {
			return err
		}
	}
	return nil
}

func validateS3DeleteResponse(output []byte) error {
	if len(strings.TrimSpace(string(output))) == 0 {
		return nil
	}
	response := struct {
		Errors []struct {
			Key     string `json:"Key"`
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Errors"`
	}{}
	if err := json.Unmarshal(output, &response); err != nil {
		return fmt.Errorf("%w: %w", errS3DeployInvalidDelete, err)
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("%w: %s", errS3DeployPartialDelete, strings.TrimSpace(string(output)))
	}
	return nil
}
