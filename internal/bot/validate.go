package bot

import (
	"fmt"
	"net/url"
	"strings"
)

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
