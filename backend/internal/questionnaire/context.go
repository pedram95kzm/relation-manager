package questionnaire

import (
	"bytes"
	"encoding/json"
	"fmt"

	"relations/internal/platform"
)

// Retaining field presence lets validation reject a module-incompatible field
// even if its submitted value is false or empty. Context values are not nullable.
func (ctx *Context) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("context must be an object")
	}
	type plain Context
	var value plain
	if err := platform.StrictJSON(data, &value); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	value.presentFields = make(map[string]bool, len(raw))
	for key, field := range raw {
		if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return fmt.Errorf("context fields cannot be null")
		}
		value.presentFields[key] = true
	}
	*ctx = Context(value)
	return nil
}
