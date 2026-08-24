package infracost

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
)

// breakdown is the minimal subset of `infracost breakdown --format json`
// output this parser needs. Unknown fields are dropped by encoding/json.
//
// Infracost emits cost values as strings (because they're high-precision
// decimals); we parse them as float64 for display.
const (
	// Bit precision used when parsing infracost cost strings via
	// strconv.ParseFloat. Matches the float64 type we store and display.
	floatBitSize = 64
	// Cap on resource-breakdown table rows — top N by monthly cost so
	// the summary stays scannable in a terminal.
	maxResourceRows = 10
)

type breakdown struct {
	Currency             string    `json:"currency"`
	Projects             []project `json:"projects"`
	TotalMonthlyCost     string    `json:"totalMonthlyCost"`
	PastTotalMonthlyCost string    `json:"pastTotalMonthlyCost"`
	DiffTotalMonthlyCost string    `json:"diffTotalMonthlyCost"`
}

type project struct {
	Name      string            `json:"name"`
	Breakdown projectBreakdown  `json:"breakdown"`
	Diff      *projectBreakdown `json:"diff,omitempty"`
}

type projectBreakdown struct {
	Resources        []resource `json:"resources"`
	TotalMonthlyCost string     `json:"totalMonthlyCost"`
}

type resource struct {
	Name         string `json:"name"`
	ResourceType string `json:"resourceType"`
	MonthlyCost  string `json:"monthlyCost"`
}

// ResultHandler reads $ATMOS_OUTPUT_FILE (infracost breakdown JSON) and
// produces a Summary with the cost diff rendered as a single markdown body.
// The same markdown is used everywhere markdown is consumed.
func ResultHandler(ctx *hooks.ExecContext) (*hooks.Summary, error) {
	if ctx == nil || ctx.OutputFile == "" {
		return nil, nil
	}
	data, err := os.ReadFile(ctx.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("%w: infracost: read output: %w", errUtils.ErrReadFile, err)
	}
	if len(data) == 0 {
		return &hooks.Summary{
			Kind:   kindName,
			Status: hooks.StatusSuccess,
			Title:  "no cost data",
			Body:   "## infracost\n\nNo cost data produced (component has no priced resources).\n",
		}, nil
	}

	var b breakdown
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("%w: infracost: parse json: %w", errUtils.ErrParseFile, err)
	}

	currency := b.Currency
	if currency == "" {
		currency = "USD"
	}

	totalDiff := parseAmount(b.DiffTotalMonthlyCost)
	totalNow := parseAmount(b.TotalMonthlyCost)
	totalPast := parseAmount(b.PastTotalMonthlyCost)

	title := formatTitle(totalDiff, currency)
	status := hooks.StatusSuccess
	// Any non-zero diff is a `warning`-level signal so the run-page card
	// flags it without failing the plan/apply.
	if totalDiff != 0 {
		status = hooks.StatusWarning
	}

	body := renderMarkdown(&b, totalNow, totalPast, totalDiff, currency)

	return &hooks.Summary{
		Kind:   kindName,
		Status: status,
		Title:  title,
		Counts: resourceCounts(&b),
		Body:   body,
	}, nil
}

// parseAmount turns infracost's string-decimal into a float64.
// Returns 0 for empty/invalid input — both are equivalent for display.
func parseAmount(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, floatBitSize)
	if err != nil {
		return 0
	}
	return v
}

func formatTitle(diff float64, currency string) string {
	if diff == 0 {
		return "no cost change"
	}
	symbol := currencySymbol(currency)
	sign := "+"
	if diff < 0 {
		sign = "-"
		diff = -diff
	}
	return fmt.Sprintf("%s%s%.2f/mo", sign, symbol, diff)
}

func currencySymbol(c string) string {
	switch strings.ToUpper(c) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	default:
		return c + " "
	}
}

// resourceCounts groups resource changes added/changed/removed. Infracost
// doesn't categorize line-by-line, so we approximate from non-zero monthly
// costs across past vs current.
func resourceCounts(b *breakdown) map[string]int {
	if b == nil {
		return nil
	}
	current := 0
	for _, p := range b.Projects {
		current += len(p.Breakdown.Resources)
	}
	if current == 0 {
		return nil
	}
	return map[string]int{"resources": current}
}

// renderMarkdown builds the single markdown rendering used for the terminal
// and downstream consumers.
func renderMarkdown(b *breakdown, now, past, diff float64, currency string) string {
	symbol := currencySymbol(currency)
	var sb strings.Builder
	fmt.Fprintf(&sb, "## infracost\n\n")

	// Headline.
	fmt.Fprintf(&sb, "| | Monthly cost |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Previous | %s%.2f |\n", symbol, past)
	fmt.Fprintf(&sb, "| Current | %s%.2f |\n", symbol, now)
	switch {
	case diff > 0:
		fmt.Fprintf(&sb, "| **Diff** | **+%s%.2f** ↑ |\n", symbol, diff)
	case diff < 0:
		fmt.Fprintf(&sb, "| **Diff** | **-%s%.2f** ↓ |\n", symbol, -diff)
	default:
		fmt.Fprintf(&sb, "| **Diff** | %s0.00 |\n", symbol)
	}

	rows := collectResourceRows(b)
	if len(rows) == 0 {
		sb.WriteString("\n_No priced resources in this component._\n")
		return sb.String()
	}

	// Sort by monthly cost descending.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].monthly > rows[j].monthly
	})

	limit := maxResourceRows
	if len(rows) < limit {
		limit = len(rows)
	}

	sb.WriteString("\n| Resource | Type | Monthly |\n|---|---|---|\n")
	for _, r := range rows[:limit] {
		fmt.Fprintf(&sb, "| %s | %s | %s%.2f |\n", r.name, r.rtype, symbol, r.monthly)
	}
	if len(rows) > limit {
		fmt.Fprintf(&sb, "\n_…and %d more resources_\n", len(rows)-limit)
	}
	return sb.String()
}

// resourceRow is a flattened cost row for rendering.
type resourceRow struct {
	name    string
	rtype   string
	monthly float64
}

// collectResourceRows flattens projects[].breakdown.resources into a single slice.
func collectResourceRows(b *breakdown) []resourceRow {
	var rows []resourceRow
	for _, p := range b.Projects {
		for _, r := range p.Breakdown.Resources {
			rows = append(rows, resourceRow{
				name:    r.Name,
				rtype:   r.ResourceType,
				monthly: parseAmount(r.MonthlyCost),
			})
		}
	}
	return rows
}
