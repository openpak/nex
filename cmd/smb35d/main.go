// Command smb35d runs the Super Mario Bros. 35 NEX server.
package main

import (
	"log"

	nexhost "openpak/nex"
	"openpak/super-mario-bros-35"
)

func main() {
	host, err := nexhost.New(nexhost.FromEnv())
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := smb35.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	server, err := smb35.New(host, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	log.Fatal(host.Run())
}
