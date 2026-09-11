package lm2

import (
	"sync"

	"github.com/openpak/nex/titles/luigis-mansion-2/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
