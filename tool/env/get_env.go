// Add package in v.1.0.2
// this package is used for utility to environment variables for getting, setting, etc...
// get_env.go is file to about getting environment

package env

import (
	"log"
	"os"
)

// get environment variable from local & occur fatal if not exist0
func GetAndFatalIfNotExits(name string) (env string) {
	if env = os.Getenv(name); env == "" {
		log.Fatalf("please set %s in environment variables", name)
	}
	return
}
