package secrets

import "github.com/cloudposse/atmos/pkg/perf"

// ValidationResult summarizes the outcome of validating a scope's declared secrets.
type ValidationResult struct {
	// MissingRequired lists required secrets that are not initialized in their backend.
	MissingRequired []Status
	// Errored lists secrets whose status could not be determined (e.g. access denied).
	Errored []Status
	// All holds the full per-secret status set.
	All []Status
}

// Valid reports whether all required secrets are initialized and no status errors occurred.
func (r ValidationResult) Valid() bool {
	defer perf.Track(nil, "secrets.ValidationResult.Valid")()

	return len(r.MissingRequired) == 0 && len(r.Errored) == 0
}

// Validate checks that every required declared secret is initialized in its backend.
func (s *Service) Validate() ValidationResult {
	defer perf.Track(s.atmosConfig, "secrets.Service.Validate")()

	// Validation is authoritative: it must contact remote backends to confirm initialization,
	// so it always verifies (verify=true). Local backends (e.g. SOPS) remain credential-free.
	statuses := s.Status(true)
	result := ValidationResult{All: statuses}
	for i := range statuses {
		st := statuses[i]
		switch {
		case st.Err != nil:
			result.Errored = append(result.Errored, st)
		case st.Declaration.Required && !st.Initialized:
			result.MissingRequired = append(result.MissingRequired, st)
		}
	}
	return result
}
