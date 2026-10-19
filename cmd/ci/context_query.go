package ci

import (
	"encoding/json"
	"fmt"

	"github.com/mikefarah/yq/v4/pkg/yqlib"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosyq "github.com/cloudposse/atmos/internal/yq"
	"github.com/cloudposse/atmos/pkg/data"
)

// writeCIContext uses Atmos's YQ engine with JSON codecs to preserve native
// payload types and large integers. Buffer evaluation before emitting any output.
func writeCIContext(value any, query string) error {
	if query == "" {
		return data.WriteJSON(value)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	atmosyq.InitExpressionParser()
	var result string
	atmosyq.WithEvaluationLevel(atmosyq.SilentLevel, func() {
		result, err = yqlib.NewStringEvaluator().Evaluate(query, string(encoded),
			yqlib.NewJSONEncoder(yqlib.JsonPreferences{Indent: 2}), yqlib.NewJSONDecoder())
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrQueryEvaluation, err)
	}
	return data.Write(result)
}
