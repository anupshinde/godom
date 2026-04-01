package env

import (
	"log"
	"os"
	"strconv"
)

var Debug = os.Getenv("GODOM_DEBUG") != ""

// Bool reads a boolean environment variable using strconv.ParseBool.
// Returns false if unset. Logs a warning if the value is not a valid bool.
func Bool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("godom: invalid value %q for %s, expected true/false or 0/1", v, key)
		return false
	}
	return b
}

// Port returns the GODOM_PORT value, or 0 if unset/invalid.
func Port() int {
	v, err := strconv.Atoi(os.Getenv("GODOM_PORT"))
	if err != nil || v == 0 {
		return 0
	}
	return v
}

// Host returns the GODOM_HOST value, or empty if unset.
func Host() string {
	host := os.Getenv("GODOM_HOST")
	if host == "" {
		host = "localhost"
	}
	return host
}

// NoAuth returns the GODOM_NO_AUTH value.
func NoAuth() bool {
	return Bool("GODOM_NO_AUTH")
}

// Token returns the GODOM_TOKEN value, or empty if unset.
func Token() string {
	return os.Getenv("GODOM_TOKEN")
}

// NoBrowser returns the GODOM_NO_BROWSER value.
func NoBrowser() bool {
	return Bool("GODOM_NO_BROWSER")
}

// Quiet returns the GODOM_QUIET value.
func Quiet() bool {
	return Bool("GODOM_QUIET")
}
