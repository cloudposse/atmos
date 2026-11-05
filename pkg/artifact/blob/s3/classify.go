package s3

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// request describes the operation that failed so the classifier can judge ambiguity.
type request struct {
	op          string
	key         string
	mutating    bool
	conditional bool
}

// Error codes S3-compatible endpoints return when they cannot honor a conditional header.
var conditionalRejectCodes = map[string]bool{
	"NotImplemented":     true,
	"XNotImplemented":    true,
	"InvalidArgument":    true,
	"InvalidRequest":     true,
	"UnsupportedHeader":  true,
	"MalformedHeader":    true,
	"HeaderNotSupported": true,
}

// classify turns an SDK error into a [blob.Error]. Nil stays nil and classified errors pass through.
func classify(req request, err error) error {
	if err == nil {
		return nil
	}
	var classified *blob.Error
	if errors.As(err, &classified) {
		return err
	}
	status, code := responseDetails(err)
	if status == 0 {
		return classifyTransport(req, err)
	}
	return blob.NewError(kindForResponse(req, status, code), req.op, req.key, err)
}

// responseDetails extracts the HTTP status and S3 error code, or zero when no response arrived.
func responseDetails(err error) (int, string) {
	var code string
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code = apiErr.ErrorCode()
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode(), code
	}
	return 0, code
}

// kindForResponse maps a service response to an error kind.
func kindForResponse(req request, status int, code string) blob.Kind {
	if kind, ok := definiteKind(status, code); ok {
		return kind
	}
	if req.conditional && rejectsCondition(status, code) {
		return blob.KindUnsupported
	}
	if req.mutating && ambiguousStatus(status) {
		return blob.KindAmbiguous
	}
	return blob.KindOther
}

// definiteKind classifies responses whose meaning does not depend on the request.
func definiteKind(status int, code string) (blob.Kind, bool) {
	switch {
	case code == "NoSuchBucket" || code == "NoSuchUpload":
		// A missing bucket or upload is a configuration or ordering problem, never a missing object.
		return blob.KindOther, true
	case isMissingObject(status, code):
		return blob.KindNotFound, true
	case isLostRace(status, code):
		return blob.KindPrecondition, true
	case status == http.StatusRequestedRangeNotSatisfiable || code == "InvalidRange":
		return blob.KindInvalid, true
	default:
		return blob.KindOther, false
	}
}

func isMissingObject(status int, code string) bool {
	return status == http.StatusNotFound || code == "NoSuchKey" || code == "NotFound"
}

// isLostRace reports a failed condition, including S3's conflict answer for concurrent conditional writes.
func isLostRace(status int, code string) bool {
	return status == http.StatusPreconditionFailed || code == "PreconditionFailed" ||
		(status == http.StatusConflict && code == "ConditionalRequestConflict")
}

// rejectsCondition reports a response that says the endpoint cannot honor a conditional header.
func rejectsCondition(status int, code string) bool {
	return status == http.StatusNotImplemented ||
		(status == http.StatusBadRequest && conditionalRejectCodes[code])
}

// ambiguousStatus reports server errors after which a mutation may or may not have been applied.
// 503 (throttling) and 501 (not implemented) are definite rejections.
func ambiguousStatus(status int) bool {
	return status >= http.StatusInternalServerError &&
		status != http.StatusServiceUnavailable &&
		status != http.StatusNotImplemented
}

// classifyTransport judges a failure with no response: a mutation whose request may have reached
// the server is ambiguous, everything else is a plain failure.
func classifyTransport(req request, err error) error {
	if req.mutating && requestMayHaveBeenSent(err) {
		return blob.NewError(blob.KindAmbiguous, req.op, req.key, err)
	}
	return blob.NewError(blob.KindOther, req.op, req.key, err)
}

// requestMayHaveBeenSent distinguishes failures after the request left the client from failures
// before it did (credential, signing, endpoint, and connection-establishment errors).
func requestMayHaveBeenSent(err error) bool {
	var send *smithyhttp.RequestSendError
	if !errors.As(err, &send) {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return false
	}
	return true
}

// unsupportedConditional builds the clear failure for an endpoint that cannot enforce a condition.
func unsupportedConditional(op, key, reason string) error {
	return blob.NewError(blob.KindUnsupported, op, key, fmt.Errorf("%w: %s", errUtils.ErrBlobConditionalUnsupported, reason))
}
