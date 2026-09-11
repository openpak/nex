package kiu

import (
	"sync"

	"github.com/openpak/nex/titles/kid-icarus-uprising/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
