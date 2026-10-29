package protected

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

// ParseKeyring accepts an explicit JSON object of key IDs to lowercase hex
// 32-byte encryption keys. It reads no environment and never echoes secrets.
func ParseKeyring(active, encoded string) (*Ring, error) {
	if len(encoded) > 8192 {
		return nil, ErrConfig
	}
	d := json.NewDecoder(strings.NewReader(encoded))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrConfig
	}
	keys := map[string][]byte{}
	for d.More() {
		raw, err := d.Token()
		if err != nil {
			return nil, ErrConfig
		}
		id, ok := raw.(string)
		if !ok || !identifier.MatchString(id) || len(keys) >= 32 {
			return nil, ErrConfig
		}
		if _, exists := keys[id]; exists {
			return nil, ErrConfig
		}
		var encodedKey string
		if d.Decode(&encodedKey) != nil || len(encodedKey) != 64 {
			return nil, ErrConfig
		}
		key, err := hex.DecodeString(encodedKey)
		if err != nil || hex.EncodeToString(key) != encodedKey {
			return nil, ErrConfig
		}
		keys[id] = key
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, ErrConfig
	}
	return New(active, keys)
}
