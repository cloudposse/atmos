package command

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/inventory"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

// row is one line of "key status".
type row struct {
	key, store, require, authority, subject, allow string
	since, expires, result, failure, identity      string
}

// keyStatus prints one row per key, from the keys, the receipts, and the identities' status files.
func (c *call) keyStatus(words []string) error {
	if err := c.flags.only(); err != nil {
		return err
	}
	if len(words) > 1 {
		return usagef("usage: key status [<name>]")
	}
	rows, err := c.statusRows()
	if err != nil {
		fmt.Fprintf(c.err, "Warning: %v\n", err)
	}
	if len(words) == 1 {
		rows = filterRows(rows, words[0])
		if len(rows) == 0 {
			return fmt.Errorf("%w: %q", ErrNoSuchKey, words[0])
		}
	}
	if len(rows) == 0 {
		c.printf("No keys, receipts, or login results on this machine.\n")
		return nil
	}
	return c.printRows(rows)
}

func filterRows(rows []row, name string) []row {
	var out []row
	for _, r := range rows {
		if r.key == name || r.identity == name {
			out = append(out, r)
		}
	}
	return out
}

func (c *call) printRows(rows []row) error {
	w := tabwriter.NewWriter(c.out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "KEY\tSTORE\tREQUIRE\tAUTHORITY\tSUBJECT\tALLOW\tSINCE\tEXPIRES\tLAST RESULT")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.key, r.store, r.require, r.authority, r.subject, r.allow, r.since, r.expires, r.result)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	for _, r := range rows {
		if r.failure != "" {
			c.printf("\nLast error for %s: %s\n", r.key, r.failure)
		}
	}
	return nil
}

// statusRows joins the three sources. A key with no receipt or status still gets a row, and so do
// a failed login whose key was never created and an enrollment whose key was deleted.
func (c *call) statusRows() ([]row, error) {
	keys, keysErr := inventory.List(c.ctx, c.svc.deps.OpenStore, c.paths)
	statuses, statusErr := statusfile.List(c.paths)
	var receipts []authority.Receipt
	if store, err := authority.NewStore(c.paths.ReceiptsDir()); err == nil {
		var listErr error
		receipts, listErr = store.List()
		statusErr = joinErrors(statusErr, listErr)
	}
	usedStatus, usedReceipt := map[int]bool{}, map[int]bool{}
	var rows []row
	for _, k := range keys {
		ri, si := matchReceipt(receipts, k), matchStatus(statuses, k)
		markUsed(usedReceipt, ri)
		markUsed(usedStatus, si)
		rows = append(rows, keyRow(k, receipts, ri, statuses, si))
	}
	for i, s := range statuses {
		if usedStatus[i] {
			continue
		}
		row := orphanStatusRow(s)
		if ri := unusedReceipt(receipts, usedReceipt, s.Key); ri >= 0 {
			usedReceipt[ri] = true
			applyReceipt(&row, receipts[ri])
			row.result = "key deleted, enrollment withdrawn at next login or logout; last: " + row.result
		}
		rows = append(rows, row)
	}
	for i, r := range receipts {
		if !usedReceipt[i] {
			rows = append(rows, orphanReceiptRow(r))
		}
	}
	return rows, joinErrors(keysErr, statusErr)
}

// unusedReceipt returns the index of a receipt for keyName that no row has used yet, or -1.
func unusedReceipt(receipts []authority.Receipt, used map[int]bool, keyName string) int {
	for i, r := range receipts {
		if !used[i] && keyName != "" && r.KeyName == keyName {
			return i
		}
	}
	return -1
}

func joinErrors(a, b error) error {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return fmt.Errorf("%w; %w", a, b)
	}
}

func markUsed(m map[int]bool, i int) {
	if i >= 0 {
		m[i] = true
	}
}

// matchReceipt returns the index of the receipt for key k, or -1.
func matchReceipt(receipts []authority.Receipt, k keystore.Info) int {
	fp := authority.Fingerprint(k.PublicKey)
	for i, r := range receipts {
		if r.KeyName == k.Name && r.Fingerprint == fp {
			return i
		}
	}
	return -1
}

// matchStatus returns the index of the newest status that names key k, or -1.
func matchStatus(statuses []statusfile.Status, k keystore.Info) int {
	best := -1
	for i, s := range statuses {
		if s.Key == k.Name && (s.Store == "" || s.Store == k.Store) && (best < 0 || s.Time.After(statuses[best].Time)) {
			best = i
		}
	}
	return best
}

func keyRow(k keystore.Info, receipts []authority.Receipt, ri int, statuses []statusfile.Status, si int) row {
	r := row{
		key: k.Name, store: k.Store, require: requireText(k.Requirements), authority: "-", subject: "-", allow: "-",
		since: "-", expires: "-", result: "no login recorded",
	}
	if ri >= 0 {
		applyReceipt(&r, receipts[ri])
	}
	if si >= 0 {
		applyStatus(&r, statuses[si])
	}
	return r
}

func applyReceipt(r *row, rec authority.Receipt) {
	r.authority = orDash(rec.Authority)
	r.subject = orDash(rec.Subject)
	r.allow = useText(rec.Uses)
	r.since = dateText(rec.Created)
	if rec.ExpiresAt.IsZero() {
		r.expires = "never"
	} else {
		r.expires = rec.ExpiresAt.UTC().Format(time.RFC3339)
	}
}

func applyStatus(r *row, s statusfile.Status) {
	r.identity = s.Identity
	r.result = resultText(s)
	r.failure = s.Error
	if r.authority == "-" {
		r.authority = orDash(s.Authority)
	}
	if r.allow == "-" && len(s.Allow) > 0 {
		r.allow = strings.Join(s.Allow, ",")
	}
}

func resultText(s statusfile.Status) string {
	out := s.Outcome
	if out == "" {
		out = "unknown"
	}
	if s.Operation == statusfile.OperationLogout {
		out = "logout: " + out
	}
	if !s.Time.IsZero() {
		out += " (" + s.Time.UTC().Format("2006-01-02 15:04") + ")"
	}
	return out
}

func orphanStatusRow(s statusfile.Status) row {
	key := s.Key
	if key == "" {
		key = s.Identity
	}
	r := row{key: key, store: orDash(s.Store), require: strings.Join(s.Require, ","), authority: "-", subject: "-", allow: "-", since: "-", expires: "-"}
	if r.require == "" {
		r.require = "-"
	}
	applyStatus(&r, s)
	return r
}

func orphanReceiptRow(rec authority.Receipt) row {
	r := row{key: rec.KeyName, store: "-", require: "-", authority: "-", subject: "-", allow: "-", since: "-", expires: "-"}
	applyReceipt(&r, rec)
	r.result = "key deleted; enrollment withdrawn at next login or logout"
	return r
}

func useText(uses []authority.Use) string {
	if len(uses) == 0 {
		return "-"
	}
	out := make([]string, len(uses))
	for i, u := range uses {
		out[i] = string(u)
	}
	return strings.Join(out, ",")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
