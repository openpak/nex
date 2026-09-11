package prw

import (
	"sync"

	"github.com/openpak/nex/titles/pokemon-rumble-world/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()
	// go nex.StartHPPServer()

	wg.Wait()
}
