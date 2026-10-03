package types

import "time"

// ExpiresWithin reports whether the credentials expire within d from now. It is cloud-agnostic and
// built on ICredentials.GetExpiration: credentials without an expiration never expire, and an
// expiration that cannot be read (GetExpiration returns an error) is treated as already expiring.
// ExpiresWithin(creds, 0) matches IsExpired for credentials whose GetExpiration mirrors IsExpired.
func ExpiresWithin(creds ICredentials, d time.Duration) bool {
	if creds == nil {
		return true
	}
	exp, err := creds.GetExpiration()
	if err != nil {
		return true
	}
	if exp == nil {
		return false
	}
	return !time.Now().Add(d).Before(*exp)
}
