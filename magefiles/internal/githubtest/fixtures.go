//go:build mage

package githubtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

type FakeAPI struct {
	T       *testing.T
	Get     func(string, any) error
	Write   func(string, string, io.Reader) error
	Archive []byte
}

func (f *FakeAPI) DoWithContext(_ context.Context, method, path string, body io.Reader, response interface{}) error {
	if method == http.MethodGet {
		return f.Get(path, response)
	}
	if f.Write == nil {
		f.T.Fatalf("unexpected write: %s %s", method, path)
	}
	return f.Write(method, path, body)
}

func (f *FakeAPI) RequestWithContext(_ context.Context, method, path string, _ io.Reader) (*http.Response, error) {
	require.Equal(f.T, http.MethodGet, method)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(f.Archive))}, nil
}

func Respond(value, response any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, response)
}

func Run() githubapi.Run {
	r := githubapi.Run{ID: 10, Attempt: 2, HeadSHA: SHA, Branch: "main", Event: "push", Status: "completed", Conclusion: "success", Path: ".github/workflows/benchmarks.yml", Created: time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC), Updated: time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC)}
	r.HeadRepository.FullName = Repo
	return r
}

//nolint:gocritic // Test callers retain their original run while mutating independent PR fixtures.
func PR(run githubapi.Run) githubapi.PR {
	p := githubapi.PR{Number: 42, State: "open"}
	p.Head.SHA = run.HeadSHA
	p.Head.Repo.FullName = run.HeadRepository.FullName
	p.Base.Ref = "main"
	p.Base.SHA = Base
	p.Base.Repo.FullName = Repo
	return p
}

const (
	Repo = "cloudposse/atmos"
	SHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	Base = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)
