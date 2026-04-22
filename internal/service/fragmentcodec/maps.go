package fragmentcodec

import (
	"encoding/json"
	"strings"
)

// EncodeOptionalMap converts a map payload into a JSON string suitable for
// Neo4j property storage. Empty maps are treated as absent values.
func EncodeOptionalMap(value map[string]any) (any, error) {
	if len(value) == 0 {
		return nil, nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

// DecodeOptionalMap converts a stored Neo4j property back into a map. It
// accepts both legacy map values and the current JSON-string encoding.
func DecodeOptionalMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		return typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(typed), &decoded); err != nil {
			return nil
		}
		if len(decoded) == 0 {
			return nil
		}
		return decoded
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		var decoded map[string]any
		if err := json.Unmarshal(typed, &decoded); err != nil {
			return nil
		}
		if len(decoded) == 0 {
			return nil
		}
		return decoded
	default:
		return nil
	}
}
