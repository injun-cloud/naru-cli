package apitypes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// IntOrPercent is the schema's "integer or N% string" for rollout
// surge/unavailable: a bare number stays an int, a percentage round-trips as
// "N%". JSON-only here (the CLI never touches the gitops YAML).
type IntOrPercent struct {
	Percent bool
	Value   int
}

func (q IntOrPercent) String() string {
	if q.Percent {
		return strconv.Itoa(q.Value) + "%"
	}
	return strconv.Itoa(q.Value)
}

func (q IntOrPercent) MarshalJSON() ([]byte, error) {
	if q.Percent {
		return json.Marshal(q.String())
	}
	return json.Marshal(q.Value)
}

func (q *IntOrPercent) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		q.Percent, q.Value = false, n
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("surge/unavailable must be an int or N%% string")
	}
	if strings.HasSuffix(s, "%") {
		v, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
		if err != nil {
			return fmt.Errorf("invalid percent %q", s)
		}
		q.Percent, q.Value = true, v
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid int-or-percent %q", s)
	}
	q.Percent, q.Value = false, v
	return nil
}
