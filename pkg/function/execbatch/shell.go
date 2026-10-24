package execbatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/utils"
)

// ShellExecutor captures the caller's working directory and environment once for a batch.
// Stderr remains masked and each line is attributed to the expression's document path.
func ShellExecutor() (Executor, error) {
	defer perf.Track(nil, "execbatch.ShellExecutor")()
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	level, err := utils.GetNextShellLevel()
	if err != nil {
		return nil, err
	}
	env := script.ProcessEnvironment(os.Environ(), map[string]string{"ATMOS_SHLVL": strconv.Itoa(level)})
	var writeMu sync.Mutex
	stderr := iolib.MaskWriter(os.Stderr)
	return func(ctx context.Context, command, path string) (any, error) {
		var stdout bytes.Buffer
		diagnostic := iolib.NewLinePrefixWriter("!exec "+path, stderr, &writeMu)
		err := process.RunShellInterpreter(ctx, &process.ShellInterpreterSpec{
			Command: command, Name: "!exec " + path, Dir: dir, Env: env,
			Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: diagnostic,
		})
		if err = errors.Join(err, diagnostic.Flush()); err != nil {
			return nil, err
		}
		text := strings.TrimRight(stdout.String(), "\n")
		var value any
		if json.Unmarshal([]byte(text), &value) == nil {
			return value, nil
		}
		return text, nil
	}, nil
}
