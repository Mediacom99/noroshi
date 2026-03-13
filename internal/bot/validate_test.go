package bot

import "testing"

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
