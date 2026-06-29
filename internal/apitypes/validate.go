package apitypes

import "fmt"

// ValidSecretKey guards a secret key the same way the server does: non-empty, no
// reserved underscore prefix, printable ASCII only. Validated client-side too so
// a bad key is rejected before the request (one rule for args, dotenv, and MCP).
func ValidSecretKey(key string) error {
	if key == "" {
		return fmt.Errorf("secret key must not be empty")
	}
	if key[0] == '_' {
		return fmt.Errorf("secret key %q must not start with underscore (reserved)", key)
	}
	for _, r := range key {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("secret key %q contains a non-printable or non-ASCII character", key)
		}
	}
	return nil
}
