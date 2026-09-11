package puyo20th

import (
	"sync"

	"github.com/openpak/nex/titles/puyo-puyo-20th/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
