package shell

import (
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/appenv"
	"runtime"
	"strconv"
)

var useGoCoreUtils bool

func init() {
	// If ATLAS-AGENT_CORE_UTILS is set to either true or false, respect that.
	// By default, enable on Windows only.
	if v, err := strconv.ParseBool(appenv.Get("CORE_UTILS")); err == nil {
		useGoCoreUtils = v
	} else {
		useGoCoreUtils = runtime.GOOS == "windows"
	}
}
