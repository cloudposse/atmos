package bridge

import (
	"bytes"
	"context"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/perf"
	apiv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/api/v1"
)

const maximumHTTPStatus = 599

// Do executes one exchange through the invocation's shared HTTP transport.
func (s *Server) Do(ctx context.Context, request *apiv1.HTTPRequest) (*apiv1.HTTPResponse, error) {
	defer perf.Track(nil, "bridge.Do")()

	invocation, err := s.authorize(ctx)
	if err != nil {
		return nil, err
	}
	defer invocation.done()
	if invocation.HTTP == nil {
		return nil, status.Error(codes.Unimplemented, "HTTP unavailable")
	}
	if len(request.GetBody()) > api.MaxHTTPBody {
		return nil, status.Error(codes.ResourceExhausted, "request exceeds limit")
	}
	req, err := http.NewRequestWithContext(invocation.Context, request.GetMethod(), request.GetUrl(), bytes.NewReader(request.GetBody()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid HTTP request")
	}
	for key, values := range request.GetHeaders() {
		req.Header[key] = append([]string(nil), values.GetValues()...)
	}
	response, err := invocation.HTTP.Do(invocation.Context, req)
	if err != nil {
		return nil, apiError(err)
	}
	if response == nil || response.Body == nil {
		return nil, status.Error(codes.Unavailable, "invalid HTTP response")
	}
	defer response.Body.Close()
	return encodeHTTPResponse(response)
}

func encodeHTTPResponse(response *http.Response) (*apiv1.HTTPResponse, error) {
	if response.StatusCode < http.StatusContinue || response.StatusCode > maximumHTTPStatus {
		return nil, status.Error(codes.Unavailable, "invalid HTTP status")
	}
	body, err := api.ReadHTTPBody(response.Body)
	if err != nil {
		return nil, apiError(err)
	}
	result := &apiv1.HTTPResponse{StatusCode: int32(response.StatusCode), Body: body, Headers: make(map[string]*apiv1.HeaderValues)}
	for key, values := range response.Header {
		result.Headers[key] = &apiv1.HeaderValues{Values: append([]string(nil), values...)}
	}
	return result, nil
}
