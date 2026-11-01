package api

import (
	"net/http"
	"os"
	"time"

	"github.com/openpak/nex/titles/super-mario-maker/database"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
)

// Start serves the internal API on SMM_API_ADDR (needs SMM_API_KEY) and the Dothub subset on
// SMM_DOTHUB_ADDR. Each is off while its variable is empty.
func Start() {
	store := &PGStore{
		DB: database.Postgres, S3: globals.MinIOClient,
		Bucket: os.Getenv("PN_SMM_CONFIG_S3_BUCKET"), KeyBase: os.Getenv("PN_SMM_CONFIG_S3_KEY_BASE"),
	}
	srv := &Server{Store: store, Key: os.Getenv("SMM_API_KEY"), Logf: globals.Logger.Infof}

	serve := func(name, addr string, h http.Handler) {
		if addr == "" {
			return
		}
		hs := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 60 * time.Second}
		go func() {
			globals.Logger.Infof("%s listening on %s", name, addr)
			if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				globals.Logger.Errorf("%s: %v", name, err)
			}
		}()
	}

	if srv.Key == "" && os.Getenv("SMM_API_ADDR") != "" {
		globals.Logger.Warning("SMM_API_ADDR is set but SMM_API_KEY is not: every request to the internal API will be refused")
	}
	serve("internal API", os.Getenv("SMM_API_ADDR"), srv.Handler())
	serve("Dothub", os.Getenv("SMM_DOTHUB_ADDR"), srv.DothubHandler())
}
