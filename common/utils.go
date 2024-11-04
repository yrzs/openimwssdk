package common

import (
	"github.com/go-kratos/kratos/v2/log"
	"runtime/debug"
)

// TryRecoverAndDebugPrint Method used to capture panic and print stack information.
func TryRecoverAndDebugPrint() {
	errs := recover()
	if errs == nil {
		return
	}
	log.Fatalf("[Panic] err:%v, stackInfo: %v", errs, debug.Stack())
}
