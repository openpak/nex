package acnl

import (
	"sync"

	"github.com/openpak/nex/titles/animal-crossing-new-leaf/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
