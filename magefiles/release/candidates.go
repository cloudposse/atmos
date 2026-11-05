package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const candidateMessagePrefix = "Atmos release candidate\n"

// candidateReservation binds a sequential tag to one execution before building.
type candidateReservation struct {
	Repository string `json:"repository"`
	Version    string `json:"version"`
	RunID      string `json:"run_id"`
	Commit     string `json:"commit"`
}

type candidateTagObject struct {
	Tag     string `json:"tag"`
	Message string `json:"message"`
	Object  struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"object"`
}

// candidateNumber accepts only canonical numeric RC suffixes for this version.
func candidateNumber(version, tag string) (uint64, bool, error) {
	prefix := version + "-rc."
	if !strings.HasPrefix(tag, prefix) {
		return 0, false, nil
	}
	suffix := strings.TrimPrefix(tag, prefix)
	if suffix == "" {
		return 0, false, nil
	}
	for _, ch := range suffix {
		if ch < '0' || ch > '9' {
			return 0, false, nil
		}
	}
	if len(suffix) > 1 && suffix[0] == '0' {
		return 0, false, nil
	}
	n, err := strconv.ParseUint(suffix, decimalBase, githubIDBits)
	if err != nil {
		return 0, false, fmt.Errorf("%w: RC number out of range: %s", ErrMilestonePolicy, tag)
	}
	return n, true, nil
}

func reservationFor(p *preparedRelease) candidateReservation {
	return candidateReservation{Repository: p.Repository, Version: p.Version, RunID: p.RunID, Commit: p.Commit}
}

// readCandidateReservation ignores historical tags without an Atmos reservation.
func (c *milestoneClient) readCandidateReservation(tag string) (*candidateReservation, error) {
	var ref candidateTagObject
	if err := c.Get("git/ref/tags/"+url.PathEscape(tag), &ref); err != nil {
		return nil, err
	}
	if ref.Object.Type != "tag" {
		return nil, nil
	}
	var object candidateTagObject
	if err := c.Get("git/tags/"+ref.Object.SHA, &object); err != nil {
		return nil, err
	}
	return parseCandidateReservation(c.Repo, tag, object)
}

func parseCandidateReservation(repo, tag string, object candidateTagObject) (*candidateReservation, error) {
	if !strings.HasPrefix(object.Message, candidateMessagePrefix) {
		return nil, nil
	}
	var reservation candidateReservation
	if err := json.Unmarshal([]byte(strings.TrimPrefix(object.Message, candidateMessagePrefix)), &reservation); err != nil {
		return nil, fmt.Errorf("%w: invalid RC reservation %s: %w", ErrReleaseVerification, tag, err)
	}
	if !reservation.matchesObject(repo, tag, object) {
		return nil, fmt.Errorf("%w: RC reservation identity mismatch: %s", ErrReleaseVerification, tag)
	}
	n, valid, err := candidateNumber(reservation.Version, tag)
	if err != nil || !valid || n == 0 {
		return nil, fmt.Errorf("%w: invalid RC reservation version: %s", ErrReleaseVerification, tag)
	}
	return &reservation, nil
}

func (r candidateReservation) matchesObject(repo, tag string, object candidateTagObject) bool {
	return object.Tag == tag && object.Object.Type == "commit" && object.Object.SHA == r.Commit &&
		githubapi.SHAPattern.MatchString(r.Commit) && canonicalRunID(r.RunID) && r.Repository == repo
}

// candidateReservations paginates tags and compares numbers, never lexical order.
func (c *milestoneClient) candidateReservations(p *preparedRelease) (uint64, string, error) {
	tags, err := milestonePages[struct {
		Name string `json:"name"`
	}](c, "tags?")
	if err != nil {
		return 0, "", err
	}
	var highest uint64
	owned := ""
	for _, tag := range tags {
		n, valid, parseErr := candidateNumber(p.Version, tag.Name)
		if parseErr != nil {
			return 0, "", parseErr
		}
		if !valid {
			continue
		}
		highest = max(highest, n)
		reservation, readErr := c.readCandidateReservation(tag.Name)
		if readErr != nil {
			return 0, "", readErr
		}
		if reservation == nil || reservation.RunID != p.RunID {
			continue
		}
		if *reservation != reservationFor(p) || owned != "" {
			return 0, "", fmt.Errorf("%w: conflicting RC reservation for run %s", ErrReleaseVerification, p.RunID)
		}
		owned = tag.Name
	}
	return highest, owned, nil
}

func (c *milestoneClient) reserveCandidate(p *preparedRelease) (string, error) {
	highest, owned, err := c.candidateReservations(p)
	if err != nil || owned != "" {
		return owned, err
	}
	if highest == math.MaxUint64 {
		return "", fmt.Errorf("%w: RC number exhausted", ErrMilestonePolicy)
	}
	tag := p.Version + "-rc." + strconv.FormatUint(highest+1, decimalBase)
	sha, err := c.createCandidateTag(p, tag)
	if err != nil {
		return "", err
	}
	writeErr := c.Write(http.MethodPost, "git/refs", map[string]string{"ref": "refs/tags/" + tag, "sha": sha})
	// A lost create-ref response may have reserved the tag. Read it back, and
	// accept only this run's exact identity; never adopt another run's tag.
	reservation, readErr := c.readCandidateReservation(tag)
	if readErr != nil {
		return "", errors.Join(writeErr, readErr)
	}
	if reservation == nil || *reservation != reservationFor(p) {
		return "", errors.Join(writeErr, fmt.Errorf("%w: RC tag %s is reserved by another execution; inspect before retrying", ErrReleaseVerification, tag))
	}
	return tag, nil
}

func (c *milestoneClient) createCandidateTag(p *preparedRelease, tag string) (string, error) {
	message, err := json.Marshal(reservationFor(p))
	if err != nil {
		return "", err
	}
	var object struct {
		SHA string `json:"sha"`
	}
	body := map[string]string{"tag": tag, "message": candidateMessagePrefix + string(message), "object": p.Commit, "type": "commit"}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	if err = c.API.DoWithContext(c.Context, http.MethodPost, "repos/"+c.Repo+"/git/tags", strings.NewReader(string(encoded)), &object); err != nil {
		return "", err
	}
	if !githubapi.SHAPattern.MatchString(object.SHA) {
		return "", fmt.Errorf("%w: invalid annotated tag SHA", ErrReleaseVerification)
	}
	return object.SHA, nil
}

func (c *milestoneClient) verifyCandidateReservation(p *preparedRelease) error {
	if p.Mode != releaseModeCandidate {
		return nil
	}
	reservation, err := c.readCandidateReservation(p.Tag)
	if err != nil {
		return err
	}
	if reservation == nil || *reservation != reservationFor(p) {
		return fmt.Errorf("%w: RC reservation does not belong to this execution", ErrReleaseVerification)
	}
	return nil
}

func preparedCandidateTag(p *preparedRelease) (string, error) {
	n, valid, err := candidateNumber(p.Version, p.Tag)
	if err != nil || !valid || n == 0 {
		return "", fmt.Errorf("%w: invalid prepared RC tag", ErrMilestonePolicy)
	}
	return p.Tag, nil
}

func (c *milestoneClient) verifyPreparedTag(p *preparedRelease) error {
	if err := c.verifyCandidateReservation(p); err != nil {
		return err
	}
	actual, err := c.tagCommit(p.Tag)
	if err != nil {
		return err
	}
	if actual != p.Commit {
		return fmt.Errorf("%w: tag moved after preparation", ErrMilestonePolicy)
	}
	return nil
}
