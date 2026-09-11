package ssb3ds

import (
	"sync"

	"github.com/openpak/nex/titles/super-smash-bros-3ds/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
