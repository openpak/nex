// Command nexd runs the shared NEX host with no title protocols registered.
//
// It is what a title server's own main looks like minus the one line that registers the
// title's protocols, and it is the thing to deploy when all you want to know is whether a
// console reaches us and gets through the handshake.
package main

import (
	"log"

	nexhost "openpak/nex"
)

func main() {
	host, err := nexhost.New(nexhost.FromEnv())
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(host.Run())
}
