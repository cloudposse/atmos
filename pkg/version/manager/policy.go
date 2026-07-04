package manager

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Update strategy values.
const (
	// StrategyMajor allows any version advance (the default).
	StrategyMajor = "major"
	// StrategyMinor allows advances within the locked major version.
	StrategyMinor = "minor"
	// StrategyPatch allows advances within the locked major.minor version.
	StrategyPatch = "patch"
	// StrategyPin never advances the version; pinned digests still refresh.
	StrategyPin = "pin"
	// StrategyDigest is an alias for StrategyPin used by digest-only entries.
	StrategyDigest = "digest"
)

// hoursPerDay and daysPerWeek support cooldown suffixes "d" and "w".
const (
	hoursPerDay = 24
	daysPerWeek = 7
)

// ErrInvalidCooldown is returned for unparseable cooldown values.
var ErrInvalidCooldown = errors.New("invalid cooldown")

// policyDecision is the outcome of applying an entry's update policy.
type policyDecision struct {
	// Target is the best candidate the policy allows.
	Target resolver.Candidate
	// Raw is the best candidate ignoring strategy and cooldown (still
	// honoring the desired expression and allow/ignore rules).
	Raw resolver.Candidate
	// Reason explains why Target differs from Raw ("" when they match).
	Reason string
}

// parseCooldown parses a cooldown duration, accepting day ("14d") and week
// ("2w") suffixes in addition to Go duration syntax ("36h").
func parseCooldown(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	if number, ok := strings.CutSuffix(value, "d"); ok {
		days, err := strconv.Atoi(number)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidCooldown, value)
		}
		return time.Duration(days) * hoursPerDay * time.Hour, nil
	}
	if number, ok := strings.CutSuffix(value, "w"); ok {
		weeks, err := strconv.Atoi(number)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidCooldown, value)
		}
		return time.Duration(weeks) * daysPerWeek * hoursPerDay * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidCooldown, value)
	}
	return duration, nil
}

// withinStrategy reports whether advancing from the locked version to the
// candidate is allowed by the strategy. An empty or unparseable locked
// version places no cap (initial lock), as does an unparseable candidate.
func withinStrategy(strategy, locked, candidate string) bool {
	switch strategy {
	case "", StrategyMajor:
		return true
	case StrategyPin, StrategyDigest:
		return candidate == locked
	}
	lockedVersion, err := semver.NewVersion(strings.TrimPrefix(locked, "v"))
	if err != nil {
		return true
	}
	candidateVersion, err := semver.NewVersion(strings.TrimPrefix(candidate, "v"))
	if err != nil {
		return true
	}
	switch strategy {
	case StrategyMinor:
		return candidateVersion.Major() == lockedVersion.Major()
	case StrategyPatch:
		return candidateVersion.Major() == lockedVersion.Major() &&
			candidateVersion.Minor() == lockedVersion.Minor()
	default:
		return true
	}
}

// cooledDown reports whether a candidate has been released for at least the
// cooldown period. Candidates without a release timestamp pass: the
// datasource cannot support cooldown, which the caller records as a reason.
func cooledDown(candidate *resolver.Candidate, cooldown time.Duration, now time.Time) bool {
	if cooldown <= 0 || candidate.ReleasedAt == nil {
		return true
	}
	return now.Sub(*candidate.ReleasedAt) >= cooldown
}

// policyInputs bundles the effective policy parameters for one decision.
type policyInputs struct {
	strategy    string
	locked      string
	cooldownRaw string
	cooldown    time.Duration
	now         time.Time
}

// decideUpdate applies an entry's effective update policy against its
// datasource candidates, deciding the version an update may advance to.
func decideUpdate(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry, locked string, now time.Time) (policyDecision, error) {
	strategy := entry.Update.Strategy
	if locked != "" && (strategy == StrategyPin || strategy == StrategyDigest) {
		return policyDecision{
			Target: resolver.Candidate{Version: locked},
			Raw:    resolver.Candidate{Version: locked},
			Reason: fmt.Sprintf("strategy %s never advances the version", strategy),
		}, nil
	}

	concrete := entry.Desired != "latest" && !resolver.LooksLikeConstraint(entry.Desired)
	res, datasource, ok := resolver.Lookup(entry.Datasource)
	if !ok || concrete {
		// Without an enumerating datasource (or with a concrete pin), the
		// desired version is both the raw and the policy target.
		candidate, err := ResolveEntry(atmosConfig, entry, false)
		if err != nil {
			return policyDecision{}, err
		}
		return policyDecision{Target: candidate, Raw: candidate}, nil
	}

	cooldown, err := parseCooldown(entry.Update.Cooldown)
	if err != nil {
		return policyDecision{}, fmt.Errorf("%s: %w", entry.Name, err)
	}
	candidates, err := res.Versions(context.Background(), resolverRequest(atmosConfig, entry, datasource))
	if err != nil {
		return policyDecision{}, err
	}
	inputs := &policyInputs{
		strategy:    strategy,
		locked:      locked,
		cooldownRaw: entry.Update.Cooldown,
		cooldown:    cooldown,
		now:         now,
	}
	return decidePolicy(entry, candidates, inputs)
}

// decidePolicy filters candidates through the strategy cap and cooldown in
// two stages so the block reasons can name both the candidate held back by
// the cap and the one held back by cooldown.
func decidePolicy(entry *EffectiveEntry, candidates []resolver.Candidate, inputs *policyInputs) (policyDecision, error) {
	raw, err := resolver.Select(candidates, entry.Desired, entry.Allow, entry.Ignore)
	if err != nil {
		return policyDecision{}, err
	}

	withinCap := filterCandidates(candidates, func(candidate *resolver.Candidate) bool {
		return inputs.locked == "" || withinStrategy(inputs.strategy, inputs.locked, candidate.Version)
	})
	capBest, capErr := resolver.Select(withinCap, entry.Desired, entry.Allow, entry.Ignore)

	cooled := filterCandidates(withinCap, func(candidate *resolver.Candidate) bool {
		return cooledDown(candidate, inputs.cooldown, inputs.now)
	})
	target, err := resolver.Select(cooled, entry.Desired, entry.Allow, entry.Ignore)
	if err != nil {
		// Nothing eligible under policy: hold the locked version.
		target = resolver.Candidate{Version: inputs.locked}
	}

	decision := policyDecision{Target: target, Raw: raw}
	decision.Reason = buildBlockReasons(inputs, &raw, &capBest, capErr, target.Version)
	return decision, nil
}

// buildBlockReasons composes the human-readable reasons for candidates the
// policy held back: the raw best blocked by the strategy cap and/or the best
// in-cap candidate blocked by cooldown.
func buildBlockReasons(inputs *policyInputs, raw, capBest *resolver.Candidate, capErr error, target string) string {
	var reasons []string
	if raw.Version != target && (capErr != nil || raw.Version != capBest.Version) {
		reasons = append(reasons, fmt.Sprintf("strategy %s holds back %s (locked %s)", inputs.strategy, raw.Version, inputs.locked))
	}
	if capErr == nil && capBest.Version != target {
		reasons = append(reasons, fmt.Sprintf("cooldown %s holds back %s (released %s)",
			inputs.cooldownRaw, capBest.Version, releasedAtString(capBest)))
	}
	return strings.Join(reasons, "; ")
}

// filterCandidates returns the candidates accepted by keep.
func filterCandidates(candidates []resolver.Candidate, keep func(*resolver.Candidate) bool) []resolver.Candidate {
	result := make([]resolver.Candidate, 0, len(candidates))
	for i := range candidates {
		if keep(&candidates[i]) {
			result = append(result, candidates[i])
		}
	}
	return result
}

// releasedAtString formats a candidate's release timestamp for messages.
func releasedAtString(candidate *resolver.Candidate) string {
	if candidate.ReleasedAt == nil {
		return "unknown"
	}
	return candidate.ReleasedAt.UTC().Format(time.RFC3339)
}
