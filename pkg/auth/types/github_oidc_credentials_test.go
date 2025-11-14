package types

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func jwtWithExp(t time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadStruct := struct {
		Exp int64 `json:"exp"`
	}{Exp: t.Unix()}
	raw, _ := json.Marshal(payloadStruct)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	// signature part is ignored by our parser; include a placeholder.
	return fmt.Sprintf("%s.%s.", header, payload)
}

func TestOIDCCredentials_GetExpiration(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	// No parts -> nil, nil.
	c := &OIDCCredentials{Token: "not-a-jwt"}
	exp, err := c.GetExpiration()
	assert.NoError(t, err)
	assert.Nil(t, exp)

	// Valid exp.
	c.Token = jwtWithExp(now.Add(42 * time.Minute))
	exp, err = c.GetExpiration()
	assert.NoError(t, err)
	if assert.NotNil(t, exp) {
		assert.WithinDuration(t, now.Add(42*time.Minute), *exp, time.Second)
	}

	// Bad base64 in payload -> decode error.
	c.Token = "aGVh.Zm9v+notbase64.sig"
	exp, err = c.GetExpiration()
	assert.Nil(t, exp)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAuthOidcDecodeFailed))

	// Bad JSON in payload -> unmarshal error.
	header := base64.RawURLEncoding.EncodeToString([]byte("{}"))
	payload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	c.Token = fmt.Sprintf("%s.%s.", header, payload)
	exp, err = c.GetExpiration()
	assert.Nil(t, exp)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAuthOidcUnmarshalFailed))
}

func TestOIDCCredentials_IsExpired_WithSkew(t *testing.T) {
	// With 5m skew, a token expiring in <5m should be considered expired now.
	soon := time.Now().UTC().Add(4*time.Minute + 30*time.Second)
	far := time.Now().UTC().Add(10 * time.Minute)

	c1 := &OIDCCredentials{Token: jwtWithExp(soon)}
	assert.True(t, c1.IsExpired())

	c2 := &OIDCCredentials{Token: jwtWithExp(far)}
	assert.False(t, c2.IsExpired())

	// No exp -> not expired.
	c3 := &OIDCCredentials{Token: "header.payload."}
	assert.False(t, c3.IsExpired())
}

func TestOIDCCredentials_BuildWhoamiInfo(t *testing.T) {
	// When GetExpiration returns a value, BuildWhoamiInfo should set it.
	texp := time.Now().UTC().Add(30 * time.Minute)
	c := &OIDCCredentials{Token: jwtWithExp(texp)}
	var w WhoamiInfo
	c.BuildWhoamiInfo(&w)
	if assert.NotNil(t, w.Expiration) {
		assert.WithinDuration(t, texp.Truncate(time.Second), w.Expiration.Truncate(time.Second), time.Second)
	}

	// Nil target is tolerated.
	c.BuildWhoamiInfo(nil)
}

func TestOIDCCredentials_Validate(t *testing.T) {
	// Validate is not implemented for OIDC credentials.
	c := &OIDCCredentials{Token: "test-token"}
	info, err := c.Validate(context.TODO())
	assert.Nil(t, info)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrNotImplemented))
}
