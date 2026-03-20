package env

import "os"

var Debug = os.Getenv("GODOM_DEBUG") != ""
