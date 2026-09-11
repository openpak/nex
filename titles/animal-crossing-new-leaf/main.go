package main

import (
	"sync"

	"github.com/PretendoNetwork/animal-crossing-new-leaf/nex"
)

var wg sync.WaitGroup

func main() {
	wg.Add(2)

	// TODO - Add gRPC server
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
