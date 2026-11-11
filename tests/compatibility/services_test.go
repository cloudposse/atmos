package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) *fixtureServer {
	t.Helper()
	s, err := newFixtureServer(filepath.Join("..", "fixtures", "compatibility"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func request(t *testing.T, client *http.Client, method, target string, headers map[string]string, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data
}

func TestHTTPFixtureAndRequestRecording(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"HEAD", "/config.json", 200}, {"GET", "/config.json?version=1", 200}, {"HEAD", "/get-only/config.json", 405}, {"HEAD", "/no-head/config.json", 501}} {
		status, body := request(t, s.Client(), tc.method, s.URL+tc.path, nil, "")
		if status != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, status)
		}
		if tc.method == "HEAD" && len(body) != 0 {
			t.Fatal("HEAD returned body")
		}
		if tc.method == "GET" && !strings.Contains(string(body), `"name":"fixture"`) {
			t.Fatalf("unexpected body %s", body)
		}
	}
	observations := s.observations()
	if len(observations) != 4 || observations[1]["path"] != "/config.json?version=1" {
		t.Fatal(observations)
	}
}

func TestFixtureTLSRejectsUnsignedAWSRequests(t *testing.T) {
	s := testServer(t)
	proxy, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(s.caPEM) {
		t.Fatal("invalid CA")
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	status, body := request(t, client, "POST", "https://"+ssmHost+"/", map[string]string{"X-Amz-Target": "AmazonSSM.GetParameter"}, `{"Name":"/compat/string"}`)
	if status != 400 || !strings.Contains(string(body), "AccessDeniedException") {
		t.Fatalf("%d %s", status, body)
	}
	observed := s.observations()
	if len(observed) != 1 || observed[0]["valid_signature"] != false {
		t.Fatal(observed)
	}
}

func TestProxyNeverForwardsExternalRequests(t *testing.T) {
	s := testServer(t)
	req, err := http.NewRequest("CONNECT", s.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "unconfigured.example:443"
	response, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 502 {
		t.Fatal(response.Status)
	}
	if got := s.observations(); len(got) != 1 || got[0]["blocked_destination"] != req.Host {
		t.Fatal(got)
	}
}

func TestConsulAuthenticationAndListingShapes(t *testing.T) {
	s := testServer(t)
	if status, _ := request(t, s.Client(), "GET", s.URL+"/v1/kv/config.json", nil, ""); status != 403 {
		t.Fatal(status)
	}
	headers := map[string]string{"X-Consul-Token": "compat-token"}
	status, body := request(t, s.Client(), "GET", s.URL+"/v1/kv/configs/?keys=", headers, "")
	var keys []string
	if err := decode(body, &keys); err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(keys, []string{"configs/app", "configs/database"}) {
		t.Fatalf("%d %v", status, keys)
	}
	status, body = request(t, s.Client(), "GET", s.URL+"/v1/kv/configs/?recurse=", headers, "")
	var pairs []map[string]any
	if err := decode(body, &pairs); err != nil {
		t.Fatal(err)
	}
	if status != 200 || len(pairs) != 2 || pairs[0]["Key"] != keys[0] || pairs[0]["Value"] != "ZnJvbnRlbmQ=" {
		t.Fatalf("%d %v", status, pairs)
	}
}

func TestVaultKV2DoesNotRouteToConsul(t *testing.T) {
	s := testServer(t)
	headers := map[string]string{"X-Vault-Token": "compat-vault-token"}
	status, body := request(t, s.Client(), "GET", s.URL+"/v1/kv/data/config", headers, "")
	if status != 200 || !strings.Contains(string(body), "vault-v2-fixture") {
		t.Fatalf("%d %s", status, body)
	}
	if got := s.observations(); len(got) != 1 || got[0]["service"] != "vault" {
		t.Fatal(got)
	}
	if status, _ := request(t, s.Client(), "LIST", s.URL+"/v1/secret/missing", headers, ""); status != 404 {
		t.Fatal("missing Vault secret unexpectedly exists", status)
	}
}
