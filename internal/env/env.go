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
