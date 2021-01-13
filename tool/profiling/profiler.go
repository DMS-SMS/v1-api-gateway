package profiling

import (
	"flag"
)

var cpuprofile = flag.String("cpuprofile", "", "Write cpu profile to file")
var memoryprofile = flag.String("memoryprofile", "", "Write memory profile to file")
var blockprofile = flag.String("blockprofile", "", "Write block profile to file")
