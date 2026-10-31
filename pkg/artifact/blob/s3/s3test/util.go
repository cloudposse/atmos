package s3test

import (
	"context"
	"encoding/xml"
	"net/http"
	"time"
)

func timeNow() time.Time {
	return time.Now()
}

// sleep pauses for d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// errorBody is the S3 XML error document.
type errorBody struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	RequestID string   `xml:"RequestId"`
}

// writeError answers with an S3 XML error. HEAD responses carry no body.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	body, _ := xml.Marshal(errorBody{Code: code, Message: message, RequestID: "s3test"})
	_, _ = w.Write(append([]byte(xml.Header), body...))
}

// writeXML answers 200 with an XML document.
func writeXML(w http.ResponseWriter, value any) {
	body, err := xml.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append([]byte(xml.Header), body...))
}
