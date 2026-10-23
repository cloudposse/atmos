//go:build mage

package milestone

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestMilestoneCreationFeedback(t *testing.T) {
	for _, mode := range []string{"created", "exists", "failure"} {
		t.Run(mode, func(t *testing.T) {
			ioCtx, err := atmosio.NewContext()
			require.NoError(t, err)
			ui.InitFormatter(ioCtx)
			t.Cleanup(ui.Reset)
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					items := []Milestone{}
					if mode == "exists" {
						items = append(items, Milestone{Number: 13, Title: "1.233.2", State: "open"})
					}
					githubtest.JSON(t, w, items)
					return
				}
				require.NotEqual(t, "exists", mode, "existing milestone must not be recreated")
				if mode == "failure" {
					http.Error(w, "denied", http.StatusForbidden)
					return
				}
				githubtest.JSON(t, w, map[string]int{"number": 13})
			})
			var out bytes.Buffer
			err = c.createMilestone("v1.233.2", ui.New(&out))
			if mode == "failure" {
				require.Error(t, err)
				require.Empty(t, out.String())
			} else {
				require.NoError(t, err)
				message := "Release milestone 1.233.2 created."
				if mode == "exists" {
					message = "Release milestone 1.233.2 already exists."
				}
				require.Equal(t, "✓ "+message+"\n", ansi.Strip(out.String()))
			}
		})
	}
}
