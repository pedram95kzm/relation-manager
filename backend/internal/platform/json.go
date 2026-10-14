package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// StrictJSON rejects ambiguous duplicate keys before decoding typed data.
// DisallowUnknownFields alone cannot detect duplicate keys discarded by maps.
func StrictJSON(data []byte, target any) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return errors.New("JSON nesting exceeds limit")
		}
		value, err := tokens.Token()
		if err != nil {
			return err
		}
		delim, ok := value.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for tokens.More() {
				key, err := tokens.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate or invalid JSON field")
				}
				seen[name] = true
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, err := tokens.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return errors.New("invalid JSON object")
			}
		case '[':
			for tokens.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, err := tokens.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return errors.New("invalid JSON array")
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}
