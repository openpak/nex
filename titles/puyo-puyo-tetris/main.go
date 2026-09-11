package ppt

import (
	"sync"

	"github.com/openpak/nex/titles/puyo-puyo-tetris/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
