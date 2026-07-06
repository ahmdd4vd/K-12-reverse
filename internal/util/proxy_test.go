package util

import (
	"strings"
	"testing"
)

func TestProxyStatusError(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantError bool
		contains  string
	}{
		{name: "ok", status: 200, wantError: false},
		{name: "redirect", status: 302, wantError: false},
		{name: "forbidden", status: 403, wantError: true, contains: "ChatGPT/OpenAI returned 403"},
		{name: "server error", status: 500, wantError: true, contains: "unexpected HTTP status 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := proxyStatusError(tt.status)
			if tt.wantError && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("expected %q in %q", tt.contains, err.Error())
			}
		})
	}
}

func TestProxyCSRFBodyError(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError bool
		contains  string
	}{
		{name: "valid csrf", body: `{"csrfToken":"abc123"}`, wantError: false},
		{name: "html body", body: `<html>blocked</html>`, wantError: true, contains: "CSRF endpoint returned non-JSON/invalid body"},
		{name: "missing token", body: `{}`, wantError: true, contains: "CSRF endpoint returned JSON without csrfToken"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := proxyCSRFBodyError([]byte(tt.body))
			if tt.wantError && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("expected %q in %q", tt.contains, err.Error())
			}
		})
	}
}
