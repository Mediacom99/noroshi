package bot

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateName checks that name contains only allowed characters and follows naming rules.
func ValidateName(name string) error {
	if len(name) == 0 || len(name) > 50 {
		return fmt.Errorf("Name must be 1-50 characters: letters, numbers, hyphens, underscores")
	}
	allDigits := true
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("Name must be 1-50 characters: letters, numbers, hyphens, underscores")
		}
		if r < '0' || r > '9' {
			allDigits = false
		}
	}
	// All-numeric names would be ambiguous with endpoint IDs in command arguments.
	if allDigits {
		return fmt.Errorf("Name must not be all numbers (it would clash with endpoint IDs)")
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("Name must not start or end with a hyphen")
	}
	return nil
}

// ValidateURL checks that raw is a well-formed HTTP(S) URL with a real-looking host.
func ValidateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a host")
	}

	if !strings.Contains(host, ".") {
		return fmt.Errorf("URL host must be a domain or IP address")
	}

	return nil
}
