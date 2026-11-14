package report

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// Result kinds. They classify an operation for the icon and for the counts of the overview.
const (
	kindSuccess   = "success"
	kindUnchanged = "unchanged"
	kindDryRun    = "dry-run"
	kindUnchecked = "unchecked"
	kindPartial   = "incomplete"
	kindFailed    = "failed"
	kindCanceled  = "canceled"
)

// kindIcons maps a result kind to its status symbol.
var kindIcons = map[string]string{
	kindSuccess:   "✅",
	kindUnchanged: "➖",
	kindDryRun:    "🔍",
	kindUnchecked: "⚠️",
	kindPartial:   "⚠️",
	kindFailed:    "❌",
	kindCanceled:  "🚫",
}

// outcomeWords holds the status wording of a successful operation by action and outcome.
var outcomeWords = map[artifact.Action]map[artifact.Outcome]string{
	artifact.ActionPublish: {
		artifact.OutcomeCreated:   "Published",
		artifact.OutcomeUnchanged: "Unchanged",
		artifact.OutcomeReplaced:  "Replaced",
		artifact.OutcomeDryRun:    "Dry run",
	},
	artifact.ActionPull: {
		artifact.OutcomeRead:      "Downloaded",
		artifact.OutcomeCreated:   "Downloaded",
		artifact.OutcomeUnchanged: "Unchanged",
		artifact.OutcomeReplaced:  "Replaced",
	},
	artifact.ActionCopy: {
		artifact.OutcomeCreated:   "Copied",
		artifact.OutcomeUnchanged: "Unchanged",
		artifact.OutcomeReplaced:  "Replaced",
	},
	artifact.ActionDelete: {
		artifact.OutcomeDeleted:   "Deleted",
		artifact.OutcomeUnchanged: "Unchanged",
	},
	artifact.ActionSign: {
		artifact.OutcomeCreated:   "Signed",
		artifact.OutcomeUnchanged: "Already signed",
	},
	artifact.ActionInspect: {artifact.OutcomeRead: "Inspected"},
	artifact.ActionShow:    {artifact.OutcomeRead: "Shown"},
	artifact.ActionList:    {artifact.OutcomeRead: "Listed"},
	artifact.ActionSearch:  {artifact.OutcomeRead: "Searched"},
	artifact.ActionPin:     {artifact.OutcomeRead: "Pinned"},
}

// mutatingActions are the actions that change repository state and so coordinate a lock.
var mutatingActions = map[artifact.Action]bool{
	artifact.ActionPublish: true,
	artifact.ActionCopy:    true,
	artifact.ActionDelete:  true,
	artifact.ActionSign:    true,
}

// knownActions lists the actions that have a section template, in documentation order.
var knownActions = []artifact.Action{
	artifact.ActionPublish, artifact.ActionPull, artifact.ActionCopy, artifact.ActionInspect,
	artifact.ActionList, artifact.ActionSearch, artifact.ActionDelete, artifact.ActionShow,
	artifact.ActionSign, artifact.ActionVerify, artifact.ActionPin,
}

// classify returns the status wording and result kind of one operation. The error decides first,
// so a partial result of a failed attempt is never reported as success.
func classify(action artifact.Action, outcome artifact.Outcome, verification *artifact.VerificationResult, err error) (word, kind string) {
	if err != nil {
		return classifyFailure(err)
	}
	if action == artifact.ActionVerify && verification != nil {
		return classifyVerification(verification.Status)
	}
	word = outcomeWords[action][outcome]
	if word == "" {
		word = titleCase(string(outcome))
	}
	if word == "" {
		word = "Completed"
	}
	switch outcome {
	case artifact.OutcomeUnchanged:
		return word, kindUnchanged
	case artifact.OutcomeDryRun:
		return word, kindDryRun
	case artifact.OutcomeFailed:
		return "Failed", kindFailed
	default:
		return word, kindSuccess
	}
}

// classifyFailure separates a canceled operation from a failed one.
func classifyFailure(err error) (word, kind string) {
	switch {
	case errors.Is(err, context.Canceled):
		return "Canceled", kindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return "Timed out", kindFailed
	default:
		return "Failed", kindFailed
	}
}

// classifyVerification words the result of a verify operation. Only a verified status claims that
// the signer was authenticated; unchecked means no trust configuration was supplied.
func classifyVerification(status artifact.VerificationStatus) (word, kind string) {
	switch status {
	case artifact.VerificationVerified:
		return "Verified", kindSuccess
	case artifact.VerificationUnchecked:
		return "Unchecked", kindUnchecked
	default:
		return "Verification failed", kindFailed
	}
}

// titleCase upper-cases the first letter of a lowercase word.
func titleCase(word string) string {
	if word == "" {
		return ""
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

// actionLabel is the title-cased action, for example Publish.
func actionLabel(action artifact.Action) string {
	if action == "" {
		return "Operation"
	}
	return titleCase(string(action))
}

// isKnownAction reports whether name is an action that has a section template.
func isKnownAction(name string) bool {
	for _, action := range knownActions {
		if string(action) == name {
			return true
		}
	}
	return false
}

// lockFor describes the lock guarantee of a finished mutation. A scope the operation did not
// report is stated as such for a successful mutation and omitted for everything else, so the
// report never invents a guarantee.
func lockFor(action artifact.Action, scope artifact.LockScope, outcome artifact.Outcome, failed bool) *Lock {
	switch scope {
	case artifact.LockScopeHost:
		return &Lock{
			Scope:     "host",
			Guarantee: "Writers on this host were serialized. Writers on other hosts are not excluded.",
		}
	case artifact.LockScopeRepository:
		return &Lock{
			Scope:     "repository",
			Guarantee: "Writers on every host were serialized through the repository's own coordination.",
		}
	}
	if failed || !mutatingActions[action] || outcome == artifact.OutcomeDryRun {
		return nil
	}
	return &Lock{Scope: "none", Guarantee: "The operation did not report a lock, so no exclusion between writers is claimed."}
}
