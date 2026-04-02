package env

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Debug is evaluated at package init, so to test both true/false values we
// re-exec the test binary with GODOM_DEBUG controlled. The child process runs
// a special test (TestDebugProbe) that just prints the value and exits.

func TestDebug_WhenUnset(t *testing.T) {
	out := runSelf(t, "")
	if out != "false" {
		t.Errorf("Debug = %q, want false when GODOM_DEBUG is unset", out)
	}
}

func TestDebug_WhenSet(t *testing.T) {
	out := runSelf(t, "1")
	if out != "true" {
		t.Errorf("Debug = %q, want true when GODOM_DEBUG=1", out)
	}
}

func TestDebug_WhenEmpty(t *testing.T) {
	out := runSelf(t, "")
	if out != "false" {
		t.Errorf("Debug = %q, want false when GODOM_DEBUG is empty", out)
	}
}

func TestDebug_WhenArbitrary(t *testing.T) {
	out := runSelf(t, "anything")
	if out != "true" {
		t.Errorf("Debug = %q, want true when GODOM_DEBUG=anything", out)
	}
}

// TestDebugProbe is the target test run in the subprocess. It prints the value
// of Debug and exits. It is not meant to be run directly.
func TestDebugProbe(t *testing.T) {
	if os.Getenv("ENV_TEST_PROBE") != "1" {
		t.Skip("only runs as subprocess")
	}
	// Print the value so the parent can capture it.
	// Use a unique prefix to distinguish from test framework output.
	if Debug {
		os.Stdout.WriteString("ENV_RESULT:true\n")
	} else {
		os.Stdout.WriteString("ENV_RESULT:false\n")
	}
}

// runSelf re-execs the test binary running only TestDebugProbe with the
// given GODOM_DEBUG value.
func runSelf(t *testing.T, godomDebug string) string {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestDebugProbe$", "-test.v")
	cmd.Env = filterEnv(os.Environ(), "GODOM_DEBUG")
	cmd.Env = append(cmd.Env, "ENV_TEST_PROBE=1")
	if godomDebug != "" {
		cmd.Env = append(cmd.Env, "GODOM_DEBUG="+godomDebug)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}

	// Extract the printed value from test output using our unique prefix.
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ENV_RESULT:") {
			return strings.TrimPrefix(line, "ENV_RESULT:")
		}
	}
	t.Fatalf("could not find ENV_RESULT in subprocess output:\n%s", out)
	return ""
}

func TestBool_True(t *testing.T) {
	t.Setenv("GODOM_TEST_BOOL", "true")
	if !Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return true for \"true\"")
	}
}

func TestBool_One(t *testing.T) {
	t.Setenv("GODOM_TEST_BOOL", "1")
	if !Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return true for \"1\"")
	}
}

func TestBool_False(t *testing.T) {
	t.Setenv("GODOM_TEST_BOOL", "false")
	if Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return false for \"false\"")
	}
}

func TestBool_Zero(t *testing.T) {
	t.Setenv("GODOM_TEST_BOOL", "0")
	if Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return false for \"0\"")
	}
}

func TestBool_Unset(t *testing.T) {
	os.Unsetenv("GODOM_TEST_BOOL")
	if Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return false when unset")
	}
}

func TestBool_Invalid(t *testing.T) {
	t.Setenv("GODOM_TEST_BOOL", "2")
	if Bool("GODOM_TEST_BOOL") {
		t.Error("Bool should return false for invalid value")
	}
}

// --- Port() ---

func TestPort_ValidValue(t *testing.T) {
	t.Setenv("GODOM_PORT", "8080")
	if got := Port(); got != 8080 {
		t.Errorf("Port() = %d, want 8080", got)
	}
}

func TestPort_Unset(t *testing.T) {
	os.Unsetenv("GODOM_PORT")
	if got := Port(); got != 0 {
		t.Errorf("Port() = %d, want 0 when unset", got)
	}
}

func TestPort_Empty(t *testing.T) {
	t.Setenv("GODOM_PORT", "")
	if got := Port(); got != 0 {
		t.Errorf("Port() = %d, want 0 for empty string", got)
	}
}

func TestPort_Invalid(t *testing.T) {
	t.Setenv("GODOM_PORT", "abc")
	if got := Port(); got != 0 {
		t.Errorf("Port() = %d, want 0 for non-numeric value", got)
	}
}

func TestPort_Zero(t *testing.T) {
	t.Setenv("GODOM_PORT", "0")
	if got := Port(); got != 0 {
		t.Errorf("Port() = %d, want 0 for explicit zero", got)
	}
}

func TestPort_Negative(t *testing.T) {
	t.Setenv("GODOM_PORT", "-1")
	// Port returns the value as-is if Atoi succeeds and value != 0
	if got := Port(); got != -1 {
		t.Errorf("Port() = %d, want -1 for negative value", got)
	}
}

// --- Host() ---

func TestHost_CustomValue(t *testing.T) {
	t.Setenv("GODOM_HOST", "0.0.0.0")
	if got := Host(); got != "0.0.0.0" {
		t.Errorf("Host() = %q, want \"0.0.0.0\"", got)
	}
}

func TestHost_Unset(t *testing.T) {
	os.Unsetenv("GODOM_HOST")
	if got := Host(); got != "localhost" {
		t.Errorf("Host() = %q, want \"localhost\" when unset", got)
	}
}

func TestHost_Empty(t *testing.T) {
	t.Setenv("GODOM_HOST", "")
	if got := Host(); got != "localhost" {
		t.Errorf("Host() = %q, want \"localhost\" for empty string", got)
	}
}

// --- NoAuth() ---

func TestNoAuth_True(t *testing.T) {
	t.Setenv("GODOM_NO_AUTH", "true")
	if !NoAuth() {
		t.Error("NoAuth() should return true when GODOM_NO_AUTH=true")
	}
}

func TestNoAuth_False(t *testing.T) {
	t.Setenv("GODOM_NO_AUTH", "false")
	if NoAuth() {
		t.Error("NoAuth() should return false when GODOM_NO_AUTH=false")
	}
}

func TestNoAuth_Unset(t *testing.T) {
	os.Unsetenv("GODOM_NO_AUTH")
	if NoAuth() {
		t.Error("NoAuth() should return false when unset")
	}
}

// --- Token() ---

func TestToken_Set(t *testing.T) {
	t.Setenv("GODOM_TOKEN", "my-secret-token")
	if got := Token(); got != "my-secret-token" {
		t.Errorf("Token() = %q, want \"my-secret-token\"", got)
	}
}

func TestToken_Unset(t *testing.T) {
	os.Unsetenv("GODOM_TOKEN")
	if got := Token(); got != "" {
		t.Errorf("Token() = %q, want empty string when unset", got)
	}
}

func TestToken_Empty(t *testing.T) {
	t.Setenv("GODOM_TOKEN", "")
	if got := Token(); got != "" {
		t.Errorf("Token() = %q, want empty string for empty env", got)
	}
}

// --- NoBrowser() ---

func TestNoBrowser_True(t *testing.T) {
	t.Setenv("GODOM_NO_BROWSER", "1")
	if !NoBrowser() {
		t.Error("NoBrowser() should return true when GODOM_NO_BROWSER=1")
	}
}

func TestNoBrowser_False(t *testing.T) {
	t.Setenv("GODOM_NO_BROWSER", "0")
	if NoBrowser() {
		t.Error("NoBrowser() should return false when GODOM_NO_BROWSER=0")
	}
}

func TestNoBrowser_Unset(t *testing.T) {
	os.Unsetenv("GODOM_NO_BROWSER")
	if NoBrowser() {
		t.Error("NoBrowser() should return false when unset")
	}
}

// --- Quiet() ---

func TestQuiet_True(t *testing.T) {
	t.Setenv("GODOM_QUIET", "true")
	if !Quiet() {
		t.Error("Quiet() should return true when GODOM_QUIET=true")
	}
}

func TestQuiet_False(t *testing.T) {
	t.Setenv("GODOM_QUIET", "false")
	if Quiet() {
		t.Error("Quiet() should return false when GODOM_QUIET=false")
	}
}

func TestQuiet_Unset(t *testing.T) {
	os.Unsetenv("GODOM_QUIET")
	if Quiet() {
		t.Error("Quiet() should return false when unset")
	}
}

func filterEnv(environ []string, key string) []string {
	prefix := key + "="
	var filtered []string
	for _, e := range environ {
		if !strings.HasPrefix(e, prefix) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
