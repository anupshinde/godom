package utils

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
)

// --- GetURLHost ---

func TestGetURLHost_PassthroughNonZero(t *testing.T) {
	// Non-0.0.0.0 hosts should be returned as-is.
	cases := []string{
		"localhost",
		"127.0.0.1",
		"192.168.1.100",
		"myhost.local",
		"",
		"0.0.0.1",
	}
	for _, host := range cases {
		got := GetURLHost(host)
		if got != host {
			t.Errorf("GetURLHost(%q) = %q, want %q", host, got, host)
		}
	}
}

func TestGetURLHost_ZeroResolves(t *testing.T) {
	// When host is "0.0.0.0", result should be either a local IP or "localhost".
	got := GetURLHost("0.0.0.0")
	if got == "0.0.0.0" {
		t.Fatal("GetURLHost(\"0.0.0.0\") should not return 0.0.0.0")
	}
	// It must be either "localhost" or a valid IPv4.
	if got == "localhost" {
		return // acceptable fallback
	}
	ip := net.ParseIP(got)
	if ip == nil || ip.To4() == nil {
		t.Errorf("GetURLHost(\"0.0.0.0\") = %q, expected localhost or valid IPv4", got)
	}
}

// --- LocalIP ---

func TestLocalIP_ValidOrEmpty(t *testing.T) {
	ip := LocalIP()
	if ip == "" {
		t.Log("LocalIP() returned empty (no non-loopback IPv4 interface found)")
		return
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("LocalIP() = %q, not a valid IP", ip)
	}
	if parsed.To4() == nil {
		t.Fatalf("LocalIP() = %q, expected IPv4", ip)
	}
	if parsed.IsLoopback() {
		t.Fatalf("LocalIP() = %q, must not be loopback", ip)
	}
}

// --- PrintQR ---

func TestPrintQR_ValidURL(t *testing.T) {
	// Capture stdout to verify it prints something and doesn't panic.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintQR("http://localhost:8080")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if len(output) == 0 {
		t.Error("PrintQR with valid URL produced no output")
	}
	// QR output uses block characters.
	if !strings.ContainsAny(output, "█▀▄ ") {
		t.Error("PrintQR output doesn't contain expected QR block characters")
	}
}

func TestPrintQR_EmptyURL(t *testing.T) {
	// Empty URL is still valid input for the QR library; should not panic.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintQR("")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	// Just verify no panic; output may or may not be empty.
}

// --- PrintUrlQRAndOpen ---

func TestPrintUrlQRAndOpen_QuietNoBrowser(t *testing.T) {
	// With quiet=true and noBrowser=true, nothing should be printed or opened.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintUrlQRAndOpen("localhost", 8080, true, "tok123", true, true)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.Len() != 0 {
		t.Errorf("Expected no output with quiet=true, got %q", buf.String())
	}
}

func TestPrintUrlQRAndOpen_PrintsURLWithAuth(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// noAuth=false → URL should include token, noBrowser=true to avoid opening browser
	PrintUrlQRAndOpen("localhost", 9090, false, "secret", true, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "http://localhost:9090?token=secret"
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, got %q", expected, output)
	}
	if !strings.Contains(output, "godom running at") {
		t.Error("Expected 'godom running at' header in output")
	}
}

func TestPrintUrlQRAndOpen_PrintsURLWithoutAuth(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// noAuth=true → URL should NOT include token
	PrintUrlQRAndOpen("127.0.0.1", 3000, true, "unused", true, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if strings.Contains(output, "token=") {
		t.Errorf("Expected no token in URL when noAuth=true, got %q", output)
	}
	expectedURL := "http://127.0.0.1:3000"
	if !strings.Contains(output, expectedURL) {
		t.Errorf("Expected output to contain %q, got %q", expectedURL, output)
	}
}

func TestPrintUrlQRAndOpen_HostPortFormatting(t *testing.T) {
	// Verify URL is formatted correctly with different host/port values.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintUrlQRAndOpen("192.168.1.5", 443, true, "", true, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "http://192.168.1.5:443"
	if !strings.Contains(output, expected) {
		t.Errorf("Expected %q in output, got %q", expected, output)
	}
}

// --- OpenBrowser ---
// [COVERAGE GAP] OpenBrowser (0% coverage)
// OpenBrowser calls exec.Command with OS-specific commands (open, xdg-open,
// rundll32) and then cmd.Start(). Testing it would launch a real browser.
// The function is not mockable without production code changes (e.g.,
// accepting an executor interface or using a package-level var for the
// command runner). The runtime.GOOS switch selects the command, and we
// cannot override runtime.GOOS in tests to hit the default/early-return path.

// [COVERAGE GAP] GetURLHost "0.0.0.0" + LocalIP()=="" path (line 17-18)
// When LocalIP returns "" (no non-loopback IPv4 interface), GetURLHost
// falls back to "localhost". This path only triggers on machines with no
// network interfaces, which is not reliably reproducible in tests without
// mocking net.InterfaceAddrs.

// [COVERAGE GAP] LocalIP error path (line 43-44)
// The err != nil return from net.InterfaceAddrs() cannot be triggered
// without mocking the net package, which requires production code changes.

// [COVERAGE GAP] PrintUrlQRAndOpen OpenBrowser call (line 38)
// The OpenBrowser(url) call inside PrintUrlQRAndOpen is only skipped when
// noBrowser=true. Testing with noBrowser=false would launch a real browser.

// --- Edge cases ---

func TestGetURLHost_ExactMatch0000(t *testing.T) {
	// Only exact "0.0.0.0" triggers resolution; partial matches should not.
	cases := []struct {
		input string
		same  bool // expect input returned unchanged
	}{
		{"0.0.0.0", false},
		{"0.0.0", true},
		{"00.0.0.0", true},
		{"0.0.0.0:8080", true}, // includes port, not exact match
	}
	for _, tc := range cases {
		got := GetURLHost(tc.input)
		if tc.same && got != tc.input {
			t.Errorf("GetURLHost(%q) = %q, expected unchanged", tc.input, got)
		}
		if !tc.same && got == tc.input {
			t.Errorf("GetURLHost(%q) should have been resolved, but was returned unchanged", tc.input)
		}
	}
}

func TestPrintQR_LongURL(t *testing.T) {
	// Long URLs should still produce QR output without panic.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	longURL := "http://example.com/" + strings.Repeat("a", 500)
	PrintQR(longURL)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	// Long URLs are valid for QR codes up to a point.
	// Just verify no panic.
	_ = buf.String()
}

func TestPrintQR_InvalidURLTooLong(t *testing.T) {
	// Extremely long input should hit the QR library's limit and return
	// early (err != nil path). Should not panic.
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	veryLong := strings.Repeat("x", 10000)
	PrintQR(veryLong)

	w.Close()
	os.Stdout = old
	// No panic = pass. This exercises the err != nil early return in PrintQR.
}

func TestPrintUrlQRAndOpen_URLConstruction(t *testing.T) {
	// Verify the exact URL construction for various param combos.
	tests := []struct {
		name    string
		host    string
		port    int
		noAuth  bool
		token   string
		wantURL string
	}{
		{"with auth", "host", 80, false, "abc", "http://host:80?token=abc"},
		{"no auth", "host", 80, true, "abc", "http://host:80"},
		{"empty token", "host", 80, false, "", "http://host:80?token="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			PrintUrlQRAndOpen(tt.host, tt.port, tt.noAuth, tt.token, true, false)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			// The first line after "godom running at\n" should be the URL
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) < 2 {
				t.Fatalf("expected at least 2 lines, got %d: %q", len(lines), output)
			}
			gotURL := lines[1]
			if gotURL != tt.wantURL {
				// The URL line might have QR data appended on same line,
				// but the fmt.Printf format puts URL on its own line.
				t.Errorf("URL line = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

// Verify PrintQR output contains newlines (one per row pair).
func TestPrintQR_OutputHasNewlines(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintQR("http://test.com")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	lines := strings.Split(output, "\n")
	// QR code for a short URL should produce multiple lines.
	if len(lines) < 5 {
		fmt.Printf("got %d lines\n", len(lines))
		t.Errorf("Expected multiple lines of QR output, got %d lines", len(lines))
	}
}
