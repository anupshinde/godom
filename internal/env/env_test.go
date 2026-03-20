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
