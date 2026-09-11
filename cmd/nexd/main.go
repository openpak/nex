// Command nexd is the shared NEX host for every OpenPak NEX title.
//
// NEX_TITLE selects the game (see titles.All); unset, it runs the bare host with no title
// protocols registered, which is the thing to deploy when all you want to know is whether a
// console reaches us and gets through the handshake. The image is one per family, the
// container is one per title.
package main

import (
	"log"
	"os"
	"sort"
	"strings"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles"
)

func main() {
	name := os.Getenv("NEX_TITLE")
	title, ok := titles.All[name]
	if !ok && name != "" && name != "nex" {
		log.Fatalf("NEX_TITLE=%q is not a title this binary knows; one of: %s", name, strings.Join(names(), ", "))
	}
	cfg := title.Config() // the zero Title is the bare host

	if title.Run != nil {
		// The title owns its listeners (Wii U / 3DS shape); nexd only adds the health endpoint.
		if err := nexhost.ServeHealth(cfg.HTTPAddr, cfg.Title); err != nil {
			log.Fatal(err)
		}
		log.Fatalf("%s: %v", title.Name, title.Run(cfg))
	}

	host, err := nexhost.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		if err := title.Register(host); err != nil {
			log.Fatalf("%s: %v", title.Name, err)
		}
	}
	log.Fatal(host.Run())
}

func names() []string {
	out := make([]string, 0, len(titles.All))
	for name := range titles.All {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
