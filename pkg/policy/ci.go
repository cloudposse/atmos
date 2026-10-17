package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Availability is explicit: an absent/zero state is never complete evidence.
type Availability string

const (
	Available   Availability = "available"
	Unavailable Availability = "unavailable"
	Incomplete  Availability = "incomplete"
)

// Provenance describes collection, not proof that the source was trusted.
type Provenance struct {
	Source      string    `json:"source"`
	CollectedAt time.Time `json:"collected_at"`
	Revision    string    `json:"revision,omitempty"`
}

// Dataset keeps native data alongside its availability, completeness, and origin.
// Adapters must not mark truncated pagination complete or fabricate event payloads.
type Dataset struct {
	Data       json.RawMessage `json:"data,omitempty"`
	Status     Availability    `json:"status"`
	Complete   bool            `json:"complete"`
	Provenance Provenance      `json:"provenance"`
	Reason     string          `json:"reason,omitempty"`
}

// CISnapshot is a small envelope with provider-owned schemas and JSON payloads.
// Triggering Event is separate from subsequently fetched Facts (reviews, checks, etc.).
// This does not replace pkg/ci.Context or the boolean workflow/hook condition ci.
type CISnapshot struct {
	Provider string             `json:"provider"`
	Schema   string             `json:"schema"`
	Metadata Dataset            `json:"metadata"`
	Event    Dataset            `json:"event"`
	Facts    map[string]Dataset `json:"facts"`
}

// CICollector is the provider adapter seam outside pure evaluation. Implementations
// must document schemas, token permissions, and capabilities. Native CI integration
// uses pkg/ci.PolicySnapshotProvider; the evaluator never invokes collectors.
type CICollector interface {
	Collect(ctx context.Context) (*CISnapshot, error)
}

// CIRequirement names an exact provider/schema and dataset required before evaluation.
// Dataset is metadata, event, or facts; Fact names the provider-specific fact for facts.
type CIRequirement struct {
	Provider string `json:"provider" yaml:"provider"`
	Schema   string `json:"schema" yaml:"schema"`
	Dataset  string `json:"dataset" yaml:"dataset"`
	Fact     string `json:"fact,omitempty" yaml:"fact,omitempty"`
}

func requireCI(snapshot *CISnapshot, requirement CIRequirement) error {
	if requirement.Provider == "" || requirement.Schema == "" {
		return fmt.Errorf("%w: CI requirement needs provider and schema", errUtils.ErrPolicyInvalid)
	}
	if snapshot == nil || snapshot.Provider != requirement.Provider || snapshot.Schema != requirement.Schema {
		return fmt.Errorf("%w: provider %q schema %q", errUtils.ErrPolicyInputUnavailable, requirement.Provider, requirement.Schema)
	}
	dataset, err := requiredDataset(snapshot, requirement)
	if err != nil {
		return err
	}
	if !completeDataset(&dataset) {
		return fmt.Errorf("%w: %s/%s", errUtils.ErrPolicyInputUnavailable, requirement.Dataset, requirement.Fact)
	}
	return nil
}

func completeDataset(dataset *Dataset) bool {
	return dataset.Status == Available && dataset.Complete && len(dataset.Data) > 0 &&
		!bytes.Equal(bytes.TrimSpace(dataset.Data), []byte("null"))
}

func requiredDataset(snapshot *CISnapshot, requirement CIRequirement) (Dataset, error) {
	if requirement.Dataset != "facts" && requirement.Fact != "" {
		return Dataset{}, fmt.Errorf("%w: fact name requires facts dataset", errUtils.ErrPolicyInvalid)
	}
	switch requirement.Dataset {
	case "metadata":
		return snapshot.Metadata, nil
	case "event":
		return snapshot.Event, nil
	case "facts":
		if requirement.Fact != "" {
			return snapshot.Facts[requirement.Fact], nil
		}
	}
	return Dataset{}, fmt.Errorf("%w: unknown CI dataset or missing fact name", errUtils.ErrPolicyInvalid)
}
