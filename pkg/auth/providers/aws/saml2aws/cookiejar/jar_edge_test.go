package cookiejar

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A host that cannot be canonicalized neither stores nor returns cookies.
func TestInvalidHostIsIgnored(t *testing.T) {
	jar := newTestJar()
	bad := &url.URL{Scheme: "https", Host: "[::1]:80:90"}
	jar.SetCookies(bad, []*http.Cookie{{Name: "sid", Value: "1"}})
	if got := jar.Cookies(bad); len(got) != 0 {
		t.Errorf("Cookies(%v) = %v, want none", bad, got)
	}
	if len(jar.entries) != 0 {
		t.Errorf("jar stored %d keys for an invalid host, want 0", len(jar.entries))
	}
}

// A cookie path that is not valid percent-encoding is kept verbatim instead of being dropped.
func TestCookiePathWithInvalidEscapeIsKept(t *testing.T) {
	jar := newTestJar()
	u := &url.URL{Scheme: "https", Host: "idp.example.com", Path: "/%zz/login"}
	jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "1", Path: "/%zz"}})
	if got := jar.Cookies(u); len(got) != 1 || got[0].Name != "sid" {
		t.Errorf("Cookies(%v) = %v, want the sid cookie", u, got)
	}
}

func TestSetCookiesWithoutCookiesIsNoop(t *testing.T) {
	jar := newTestJar()
	jar.SetCookies(mustParseURL("https://idp.example.com/"), nil)
	if len(jar.entries) != 0 {
		t.Errorf("jar stored %d keys, want 0", len(jar.entries))
	}
}

// A domain attribute on an IP host is rejected, so nothing is stored.
func TestSetCookiesRejectsDomainOnIPHost(t *testing.T) {
	jar := newTestJar()
	u := mustParseURL("https://127.0.0.1/")
	jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "1", Domain: "example.com"}})
	if got := jar.Cookies(u); len(got) != 0 {
		t.Errorf("Cookies(%v) = %v, want none", u, got)
	}
}

// When the last persistent cookie of a host expires, its entry map is dropped.
func TestExpiredCookiesDropHostEntry(t *testing.T) {
	jar := newTestJar()
	u := mustParseURL("https://idp.example.com/")
	jar.setCookies(u, []*http.Cookie{{Name: "sid", Value: "1", MaxAge: 60}}, tNow)
	if len(jar.entries) != 1 {
		t.Fatalf("jar has %d keys, want 1", len(jar.entries))
	}
	if got := jar.cookies(u, tNow.Add(2*time.Minute)); len(got) != 0 {
		t.Errorf("expired cookies = %v, want none", got)
	}
	if len(jar.entries) != 0 {
		t.Errorf("jar has %d keys after expiry, want 0", len(jar.entries))
	}
}

// Cookies with equal path lengths are ordered by creation time (RFC 6265 section 5.4).
func TestCookiesOrderedByCreationTime(t *testing.T) {
	jar := newTestJar()
	u := mustParseURL("https://idp.example.com/")
	jar.setCookies(u, []*http.Cookie{{Name: "second", Value: "2"}}, tNow.Add(time.Second))
	jar.setCookies(u, []*http.Cookie{{Name: "first", Value: "1"}}, tNow)
	got := jar.cookies(u, tNow.Add(time.Minute))
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Errorf("cookies = %v, want [first second]", got)
	}
}

func TestToASCII(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"golang", "golang"},
		{"bücher.example.com", "xn--bcher-kva.example.com"},
		{"ü", "xn--tda"},
		// RFC 3492 section 7.1 sample strings (long labels exercise the digit and bias logic).
		{"ليهمابتكلموشعربي؟", "xn--egbpdaj6bu4bxfgehfvwxn"},
		{"他们为什么不说中文", "xn--ihqwcrb4cv8a8dqg056pqjye"},
		{"Pročprostěnemluvíčesky", "xn--Proprostnemluvesky-uyb24dma41a"},
		{"なぜみんな日本語を話してくれないのか", "xn--n8jok5ay5dzabd5bym9f0cm5685rrjetr6pdxa"},
	}
	for _, tt := range tests {
		got, err := toASCII(tt.in)
		if err != nil {
			t.Errorf("toASCII(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("toASCII(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A label whose code point delta overflows is rejected instead of encoded incorrectly.
func TestToASCIIRejectsOverflowingLabel(t *testing.T) {
	label := strings.Repeat("a", 3000) + "\U0010FFFF"
	if got, err := toASCII(label); err == nil {
		t.Errorf("toASCII(long label) = %q, want an error", got)
	}
}

func TestEncodeDigitPanicsOutOfRange(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("encodeDigit(36) did not panic")
		}
	}()
	encodeDigit(36)
}

// Only HTTP and HTTPS URLs take part in the jar.
func TestNonHTTPSchemeIsIgnored(t *testing.T) {
	jar := newTestJar()
	u := mustParseURL("ftp://idp.example.com/")
	jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "1"}})
	if len(jar.entries) != 0 {
		t.Errorf("jar stored %d keys for an ftp URL, want 0", len(jar.entries))
	}
	if got := jar.Cookies(u); len(got) != 0 {
		t.Errorf("Cookies(%v) = %v, want none", u, got)
	}
}

// Regression: an IPv6 literal was stored without brackets when the URL had a port and looked up with brackets
// when it had none, so the cookie was lost across such a redirect.
func TestIPv6HostSameKeyWithAndWithoutPort(t *testing.T) {
	jar := newTestJar()
	jar.SetCookies(mustParseURL("https://[2001:db8::1]:443/login"), []*http.Cookie{{Name: "sid", Value: "1"}})

	for _, target := range []string{"https://[2001:db8::1]/continue", "https://[2001:db8::1]:443/continue"} {
		if got := jar.Cookies(mustParseURL(target)); len(got) != 1 || got[0].Name != "sid" {
			t.Errorf("Cookies(%s) = %v, want the sid cookie", target, got)
		}
	}

	// A cookie set on the portless form is found on the ported form too.
	jar = newTestJar()
	jar.SetCookies(mustParseURL("https://[2001:db8::1]/login"), []*http.Cookie{{Name: "sid", Value: "2"}})
	if got := jar.Cookies(mustParseURL("https://[2001:db8::1]:8443/continue")); len(got) != 1 {
		t.Errorf("Cookies on the ported form = %v, want the sid cookie", got)
	}
}
