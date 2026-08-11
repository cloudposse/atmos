package pro

import (
	"bytes"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// UploadExecMetadata uploads a single command-execution record to Atmos Pro.
// Each execution record is one indivisible logical unit (unlike affected-stacks
// uploads), so it is never chunked — oversized payloads are truncated client-side
// before this method is called (see pkg/proexec/truncate.go).
func (c *AtmosProAPIClient) UploadExecMetadata(dto *dtos.ExecUploadRequest) error {
	url := fmt.Sprintf("%s/%s/atmos/exec", c.BaseURL, c.BaseAPIEndpoint)

	data, err := json.Marshal(dto)
	if err != nil {
		return wrapErr(errUtils.ErrFailedToMarshalPayload, err)
	}

	log.Debug("Uploading command-execution metadata.", logKeyURL, url)

	err = doWithRetry("UploadExecMetadata", func() error {
		req, reqErr := getAuthenticatedRequest(c, "POST", url, bytes.NewBuffer(data))
		if reqErr != nil {
			return wrapErr(errUtils.ErrFailedToCreateAuthRequest, reqErr)
		}

		resp, doErr := c.HTTPClient.Do(req) //nolint:gosec // URL constructed from trusted config, not user input.
		if doErr != nil {
			return wrapErr(errUtils.ErrFailedToMakeRequest, doErr)
		}
		defer resp.Body.Close()

		return handleAPIResponse(resp, "UploadExecMetadata")
	}, c, defaultRetryConfig())
	if err != nil {
		return wrapErr(errUtils.ErrFailedToUploadExecMetadata, err)
	}

	log.Debug("Uploaded command-execution metadata.", logKeyURL, url)

	return nil
}
