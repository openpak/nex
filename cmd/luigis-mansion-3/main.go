// Command luigis-mansion-3 runs the OpenPak NEX server for Luigi's Mansion 3.
package main

import (
	"log"

	lm3 "github.com/openpak/luigis-mansion-3"
)

func main() {
	log.Fatal(lm3.Run(lm3.FromEnv()))
}
