package mh3u

import (
	"sync"

	"github.com/openpak/nex/titles/monster-hunter-3-ultimate/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
