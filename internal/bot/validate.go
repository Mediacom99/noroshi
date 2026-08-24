package bot

import (
	"fmt"
	"net/url"
	"regexp"
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

// ValidateURL checks that raw is a well-formed monitor target URL.
// Supported schemes: http/https (dotted host required), tcp (host:port),
// dns and ping (host or IP; single-label hosts allowed for internal services).
func ValidateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a host")
	}

	switch parsed.Scheme {
	case "http", "https":
		if !strings.Contains(host, ".") {
			return fmt.Errorf("URL host must be a domain or IP address")
		}
	case "tcp":
		if parsed.Port() == "" {
			return fmt.Errorf("tcp:// URL must include a port (e.g. tcp://db:5432)")
		}
	case "dns", "ping":
		// Host alone is the target.
	default:
		return fmt.Errorf("URL scheme must be http, https, tcp, dns or ping")
	}

	return nil
}

// ValidateKeywordSpec checks a /keyword spec: plain substring, "!" prefix for
// absence, "re:" prefix for a regexp (which must compile), "!re:" for a
// negated regexp. An empty effective spec is rejected.
func ValidateKeywordSpec(keyword string) error {
	spec := strings.TrimPrefix(keyword, "!")
	if pattern, ok := strings.CutPrefix(spec, "re:"); ok {
		if pattern == "" {
			return fmt.Errorf("Regex must not be empty")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("Invalid regex: %w", err)
		}
		return nil
	}
	if spec == "" {
		return fmt.Errorf("Keyword must not be empty")
	}
	return nil
}

// maintDayCodes are the accepted lowercase day abbreviations.
var maintDayCodes = map[string]bool{
	"mon": true, "tue": true, "wed": true, "thu": true,
	"fri": true, "sat": true, "sun": true,
}

// ParseMaintDays normalizes a /maint days argument: "all" or a comma-separated
// list of day codes (mon..sun, case-insensitive). Returns the canonical
// lowercase form.
func ParseMaintDays(arg string) (string, error) {
	arg = strings.ToLower(strings.TrimSpace(arg))
	if arg == "all" {
		return "all", nil
	}
	var days []string
	for _, d := range strings.Split(arg, ",") {
		d = strings.TrimSpace(d)
		if !maintDayCodes[d] {
			return "", fmt.Errorf("Invalid day %q — use all or mon,tue,wed,thu,fri,sat,sun", d)
		}
		days = append(days, d)
	}
	if len(days) == 0 {
		return "", fmt.Errorf("Days must be all or mon,tue,wed,thu,fri,sat,sun")
	}
	return strings.Join(days, ","), nil
}

// ParseMaintTimeRange parses "HH:MM-HH:MM" into minutes since midnight UTC.
// Start may be later than end (overnight window); equal start and end is
// rejected as ambiguous (zero-length vs 24h).
func ParseMaintTimeRange(arg string) (start, end int, err error) {
	parts := strings.Split(arg, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("Time range must be HH:MM-HH:MM (e.g. 02:00-04:00)")
	}
	start, err = parseHHMM(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err = parseHHMM(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if start == end {
		return 0, 0, fmt.Errorf("Start and end time must differ")
	}
	return start, end, nil
}

func parseHHMM(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("Invalid time %q — use HH:MM", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("Invalid time %q — use HH:MM (00:00-23:59)", s)
	}
	return h*60 + m, nil
}
