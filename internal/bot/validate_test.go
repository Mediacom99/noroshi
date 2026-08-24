package bot

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"valid with path", "http://sub.example.com/path", false},
		{"valid with port", "https://example.com:8080/path?q=1", false},
		{"valid IP", "http://1.2.3.4", false},
		{"valid IP with port", "http://192.168.1.1:8080", false},
		{"empty string", "", true},
		{"no scheme", "example.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "http://", true},
		{"no dot in host", "http://localhost", true},
		{"no dot bare word", "http://foo", true},
		{"scheme only", "https", true},
		{"valid tcp", "tcp://db.internal:5432", false},
		{"valid tcp IP", "tcp://192.168.1.10:5432", false},
		{"tcp missing port", "tcp://db.internal", true},
		{"valid dns", "dns://example.com", false},
		{"valid dns single label", "dns://nas", false},
		{"valid ping", "ping://192.168.1.1", false},
		{"dns no host", "dns://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid lowercase with hyphen", "prod-api", false},
		{"valid mixed case", "MyAPI", false},
		{"valid underscore and digit", "site_1", false},
		{"valid single char", "a", false},
		{"valid hyphen in middle", "a-b", false},
		{"valid at 50 chars", strings.Repeat("a", 50), false},
		{"valid underscore at start", "_ok", false},
		{"empty string", "", true},
		{"too long 51 chars", strings.Repeat("a", 51), true},
		{"starts with hyphen", "-api", true},
		{"ends with hyphen", "api-", true},
		{"contains space", "prod api", true},
		{"contains dot", "prod.api", true},
		{"contains at sign", "prod@api", true},
		{"contains slash", "a/b", true},
		{"all numeric", "123", true},
		{"all numeric long", "1234567890", true},
		{"numeric with letter", "123a", false},
		{"numeric with hyphen", "123-api", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateKeywordSpec(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		wantErr bool
	}{
		{"plain substring", `"status":"ok"`, false},
		{"negated substring", "!fatal error", false},
		{"regex", `re:version-[0-9]+`, false},
		{"negated regex", `!re:error|exception`, false},
		{"invalid regex", "re:[unclosed", true},
		{"invalid negated regex", "!re:(", true},
		{"empty regex", "re:", true},
		{"empty negated regex", "!re:", true},
		{"bare negation", "!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeywordSpec(tt.keyword)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKeywordSpec(%q) error = %v, wantErr %v", tt.keyword, err, tt.wantErr)
			}
		})
	}
}
