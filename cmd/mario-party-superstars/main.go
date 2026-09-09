// Command mario-party-superstars runs the OpenPak NEX server for Mario Party Superstars.
package main

import (
	"log"

	mps "openpak/mario-party-superstars"
)

func main() {
	log.Fatal(mps.Run(mps.FromEnv()))
}
