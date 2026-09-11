package smmwiiu

import (
	"sync"

	"github.com/openpak/nex/titles/super-mario-maker/grpc"
	"github.com/openpak/nex/titles/super-mario-maker/nex"
)

var wg sync.WaitGroup

func run() {
	wg.Add(2)

	go grpc.StartGRPCServer()
	go nex.StartAuthenticationServer()
	go nex.StartSecureServer()

	wg.Wait()
}
