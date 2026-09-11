package mpff

import (
	"sync"

	"github.com/openpak/nex/titles/metroid-prime-federation-force/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
