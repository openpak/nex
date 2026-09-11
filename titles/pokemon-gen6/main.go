package pokegen6

import (
	"sync"

	"github.com/openpak/nex/titles/pokemon-gen6/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
