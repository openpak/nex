package remerc3d

import (
	"sync"

	"github.com/openpak/nex/titles/resident-evil-the-mercenaries-3d/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
