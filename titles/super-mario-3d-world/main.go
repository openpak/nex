package sm3dwu

import (
	"sync"

	"github.com/openpak/nex/titles/super-mario-3d-world/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
