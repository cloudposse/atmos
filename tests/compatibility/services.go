package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	accessKey = "compat-access-key"
	secretKey = "compat-secret-key"
	ssmHost   = "ssm.us-east-1.amazonaws.com"
)

type fixtureServer struct {
	*httptest.Server
	fixtures  string
	caPEM     []byte
	tlsConfig *tls.Config
	mu        sync.Mutex
	requests  []map[string]any
}

func newFixtureServer(fixtures string) (*fixtureServer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Atmos compatibility fixture"},
		DNSNames: []string{ssmHost}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	server := &fixtureServer{
		fixtures: fixtures, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		requests:  []map[string]any{},
	}
	server.Server = httptest.NewServer(http.HandlerFunc(server.serveHTTP))
	return server, nil
}

func (s *fixtureServer) record(request map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
}

func (s *fixtureServer) reset() { s.mu.Lock(); defer s.mu.Unlock(); s.requests = []map[string]any{} }

func (s *fixtureServer) observations() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any{}, s.requests...)
}

func header(r *http.Request, name string) any {
	value := r.Header.Get(name)
	if value == "" {
		return nil
	}
	return value
}

func reply(w http.ResponseWriter, r *http.Request, status int, value any, contentType string, headers map[string]string) {
	data, ok := value.([]byte)
	if !ok {
		data = []byte(canonical(value))
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Header().Set("X-Amzn-RequestId", "fixture-request")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func jsonReply(w http.ResponseWriter, r *http.Request, status int, value any) {
	reply(w, r, status, value, "application/json", nil)
}

// connect terminates only the fixture's SSM TLS tunnel. It never forwards traffic.
func (s *fixtureServer) connect(w http.ResponseWriter, r *http.Request) {
	if r.Host != ssmHost+":443" {
		s.record(map[string]any{"blocked_destination": r.Host})
		http.Error(w, "fixture refuses external destinations", 502)
		return
	}
	connection, buffer, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if err := buffer.Flush(); err != nil {
		return
	}
	secure := tls.Server(connection, s.tlsConfig)
	defer secure.Close()
	reader := bufio.NewReader(secure)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	defer request.Body.Close()
	recorder := httptest.NewRecorder()
	s.serveHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	response.Close = true
	_ = response.Write(secure)
}

func signatureValid(r *http.Request, body []byte, service string) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		return false
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(auth, "AWS4-HMAC-SHA256 "), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return false
		}
		fields[key] = value
	}
	scope := strings.Split(fields["Credential"], "/")
	if len(scope) != 5 || scope[0] != accessKey || scope[2] != "us-east-1" || scope[3] != service || scope[4] != "aws4_request" {
		return false
	}
	var headers strings.Builder
	for _, name := range strings.Split(fields["SignedHeaders"], ";") {
		value := r.Header.Get(name)
		if name == "host" {
			value = r.Host
		}
		headers.WriteString(name + ":" + strings.Join(strings.Fields(value), " ") + "\n")
	}
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
	}
	canonicalRequest := strings.Join([]string{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, headers.String(), fields["SignedHeaders"], payloadHash}, "\n")
	sum := sha256.Sum256([]byte(canonicalRequest))
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", r.Header.Get("X-Amz-Date"), strings.Join(scope[1:], "/"), hex.EncodeToString(sum[:])}, "\n")
	key := []byte("AWS4" + secretKey)
	for _, part := range scope[1:] {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(part))
		key = mac.Sum(nil)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(toSign))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(fields["Signature"]))
}

func (s *fixtureServer) ssm(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	params := map[string]any{}
	if err := decode(body, &params); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	target := r.Header.Get("X-Amz-Target")
	valid := signatureValid(r, body, "ssm")
	s.record(map[string]any{"service": "ssm", "method": "POST", "target": target, "body": params, "valid_signature": valid, "session_token": header(r, "X-Amz-Security-Token")})
	name, _ := params["Name"].(string)
	if !valid || strings.Contains(name, "denied") {
		jsonReply(w, r, 400, map[string]any{"__type": "AccessDeniedException", "message": "fixture access denied"})
		return
	}
	values := map[string][2]string{
		"/compat/string": {"String", "fixture-value"}, "/compat/secure": {"SecureString", "fixture-secret-value"},
		"/compat/json": {"String", `{"count":42,"enabled":true,"items":["a","b"]}`}, "/compat/stringlist": {"StringList", "a,b,c"},
		"/compat/list/a": {"String", "first"}, "/compat/list/b": {"String", "second"},
	}
	parameter := func(name string) map[string]any {
		return map[string]any{"Name": name, "Type": values[name][0], "Value": values[name][1], "Version": 1, "DataType": "text", "ARN": "arn:aws:ssm:us-east-1:123456789012:parameter" + name}
	}
	switch {
	case strings.HasSuffix(target, ".GetParameter"):
		if _, ok := values[name]; !ok {
			jsonReply(w, r, 400, map[string]any{"__type": "ParameterNotFound", "message": "fixture parameter not found"})
		} else {
			jsonReply(w, r, 200, map[string]any{"Parameter": parameter(name)})
		}
	case strings.HasSuffix(target, ".GetParametersByPath"):
		path, _ := params["Path"].(string)
		names := []string{}
		for name := range values {
			if strings.HasPrefix(name, strings.TrimRight(path, "/")+"/") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		parameters := []any{}
		for _, name := range names {
			parameters = append(parameters, parameter(name))
		}
		jsonReply(w, r, 200, map[string]any{"Parameters": parameters})
	default:
		s.record(map[string]any{"unsupported_operation": target})
		jsonReply(w, r, 400, map[string]any{"__type": "UnknownOperationException"})
	}
}

func (s *fixtureServer) consul(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/v1/kv/") {
		return false
	}
	token := header(r, "X-Consul-Token")
	s.record(map[string]any{"service": "consul", "method": r.Method, "path": r.URL.RequestURI(), "token": token})
	if token != "compat-token" {
		reply(w, r, 403, []byte("fixture permission denied"), "text/plain", nil)
		return true
	}
	key := strings.TrimPrefix(r.URL.Path, "/v1/kv/")
	values := map[string]string{"config.json": `{"name":"consul-fixture","count":42}`, "configs/app": "frontend", "configs/database": "postgres", "plain": "plain-value"}
	matching := []string{}
	for name := range values {
		if strings.HasPrefix(name, key) {
			matching = append(matching, name)
		}
	}
	sort.Strings(matching)
	query := r.URL.Query()
	if query.Has("keys") {
		jsonReply(w, r, 200, matching)
	} else if _, ok := values[key]; query.Has("recurse") || ok {
		if !query.Has("recurse") {
			matching = []string{key}
		}
		pairs := []any{}
		for _, name := range matching {
			pairs = append(pairs, map[string]any{"Key": name, "Value": base64.StdEncoding.EncodeToString([]byte(values[name])), "CreateIndex": 1, "ModifyIndex": 1, "LockIndex": 0, "Flags": 0})
		}
		jsonReply(w, r, 200, pairs)
	} else {
		reply(w, r, 404, []byte("fixture key not found"), "text/plain", nil)
	}
	return true
}

func (s *fixtureServer) s3(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/compat-bucket") {
		return false
	}
	valid := signatureValid(r, nil, "s3")
	s.record(map[string]any{"service": "s3", "method": r.Method, "path": r.URL.RequestURI(), "valid_signature": valid, "range": header(r, "Range")})
	if !valid {
		reply(w, r, 403, []byte("<Error><Code>AccessDenied</Code></Error>"), "application/xml", nil)
		return true
	}
	key := strings.TrimLeft(strings.TrimPrefix(r.URL.Path, "/compat-bucket"), "/")
	content := []byte(`{"name":"s3-fixture","count":42}`)
	if r.URL.Query().Has("list-type") {
		payload := fmt.Sprintf(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>compat-bucket</Name><IsTruncated>false</IsTruncated><Contents><Key>config.json</Key><Size>%d</Size><LastModified>2020-01-01T00:00:00Z</LastModified><ETag>"fixture"</ETag><StorageClass>STANDARD</StorageClass></Contents></ListBucketResult>`, len(content))
		reply(w, r, 200, []byte(payload), "application/xml", nil)
	} else if key == "config.json" || key == "corrupt.json" {
		crc := crc32.ChecksumIEEE(content)
		if key == "corrupt.json" {
			crc = 0
		}
		checksum := make([]byte, 4)
		binary.BigEndian.PutUint32(checksum, crc)
		reply(w, r, 200, content, "application/json", map[string]string{"X-Amz-Checksum-Crc32": base64.StdEncoding.EncodeToString(checksum), "Last-Modified": "Wed, 01 Jan 2020 00:00:00 GMT"})
	} else {
		reply(w, r, 404, []byte("<Error><Code>NoSuchKey</Code></Error>"), "application/xml", nil)
	}
	return true
}

func (s *fixtureServer) vault(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/v1/secret") && !strings.HasPrefix(path, "/v1/kv/") && !strings.HasPrefix(path, "/v1/sys/") && !strings.HasPrefix(path, "/v1/auth/") {
		return false
	}
	token := header(r, "X-Vault-Token")
	s.record(map[string]any{"service": "vault", "method": r.Method, "path": r.URL.RequestURI(), "token": token, "namespace": header(r, "X-Vault-Namespace")})
	switch {
	case token != "compat-vault-token":
		jsonReply(w, r, 403, map[string]any{"errors": []string{"fixture permission denied"}})
	case path == "/v1/sys/internal/ui/mounts":
		jsonReply(w, r, 200, map[string]any{"data": map[string]any{"secret": map[string]any{"secret/": map[string]any{"type": "kv", "options": map[string]string{"version": "1"}}, "kv/": map[string]any{"type": "kv", "options": map[string]string{"version": "2"}}}}})
	case (r.Method == "LIST" || r.URL.Query().Has("list")) && (strings.TrimRight(path, "/") == "/v1/secret" || strings.TrimRight(path, "/") == "/v1/kv/metadata"):
		jsonReply(w, r, 200, map[string]any{"data": map[string]any{"keys": []string{"config"}}})
	case path == "/v1/secret/config":
		jsonReply(w, r, 200, map[string]any{"data": map[string]any{"name": "vault-fixture", "count": 42}})
	case path == "/v1/kv/data/config":
		jsonReply(w, r, 200, map[string]any{"data": map[string]any{"data": map[string]any{"name": "vault-v2-fixture"}, "metadata": map[string]any{"created_time": "2020-01-01T00:00:00Z", "version": 1, "destroyed": false}}})
	default:
		jsonReply(w, r, 404, map[string]any{"errors": []string{}})
	}
	return true
}

func (s *fixtureServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodConnect:
		s.connect(w, r)
		return
	case http.MethodPost:
		s.ssm(w, r)
		return
	case "LIST":
		if !s.vault(w, r) {
			http.NotFound(w, r)
		}
		return
	}
	// Vault KV-v2 and Consul share /v1/kv/. Use the client's header to disambiguate.
	if r.Header.Get("X-Vault-Token") != "" && s.vault(w, r) {
		return
	}
	if s.consul(w, r) || s.s3(w, r) || s.vault(w, r) {
		return
	}
	s.record(map[string]any{"method": r.Method, "path": r.URL.RequestURI(), "accept": header(r, "Accept"), "fixture_header": header(r, "X-Fixture")})
	if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/get-only/") {
		w.WriteHeader(405)
		return
	}
	if r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, "/no-head/") {
		w.WriteHeader(501)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/slow/") {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/redirect/") {
		w.Header().Set("Location", "/config.json")
		w.WriteHeader(302)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/protected/") && r.Header.Get("X-Fixture") != "compatibility" {
		reply(w, r, 401, []byte("fixture unauthorized"), "text/plain", nil)
		return
	}
	name, err := url.PathUnescape(filepath.Base(r.URL.EscapedPath()))
	if err != nil || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.fixtures, "data", name))
	if err != nil {
		reply(w, r, 404, []byte("fixture not found\n"), "text/plain", nil)
		return
	}
	contentType := "text/plain"
	switch filepath.Ext(name) {
	case ".json":
		contentType = "application/json"
	case ".yaml":
		contentType = "application/yaml"
	}
	reply(w, r, 200, data, contentType, nil)
}
