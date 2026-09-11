package tkcd

import (
	"sync"

	"github.com/openpak/nex/titles/team-kirby-clash-deluxe/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
