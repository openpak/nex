package nexhost

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/openpak/nex/internal/coreid"
	"github.com/openpak/nex/internal/coreranking"
	lbpb "github.com/openpak/nex/internal/leaderboardspb"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// The stores a title can move to the core, one name each in NEX_CORE_STORAGE
// (docs/core-backed-storage.md). Off by default: a title keeps its own tables until its
// container's environment names the store.
const (
	CoreRankingsStore  = "rankings"
	CoreDataStoreStore = "datastore" // not built yet: core gaps (docs/core-backed-storage.md)
	CoreMessagingStore = "messaging" // not built yet: core gaps
)

// CoreStorage reports whether NEX_CORE_STORAGE (comma separated) names store.
func CoreStorage(store string) bool {
	for _, s := range strings.Split(os.Getenv("NEX_CORE_STORAGE"), ",") {
		if strings.EqualFold(strings.TrimSpace(s), store) {
			return true
		}
	}
	return false
}

// CoreTitle is how a title is known to the core services.
type CoreTitle struct {
	Key     string // neutral key = native id, the NEX_TITLE name ("mario-kart-8")
	Name    string // display name
	Network string // "wiiu" or "3ds": the account link namespace and the core network
	Kind    string // rankingsapi.KindTime or KindScore
	Order   string // "asc", "desc" or "" (each board from its first score's OrderBy)
}

// NewCoreRankings builds the core-backed ranking store of a title from the environment:
//
//	NEX_LEADERBOARDS_ADDR    leaderboards gRPC (host:port); key NEX_LEADERBOARDS_KEY or LEADERBOARDS_INTERNAL_KEY
//	NEX_TITLE_STORAGE_ADDR   title-storage gRPC; key NEX_TITLE_STORAGE_KEY or TITLE_STORAGE_INTERNAL_KEY
//	NEX_ACCOUNT_GRPC_ADDR/_KEY  nn-account (PID <-> account), from cfg
//
// db is the title database (pending rows); nil makes a write the core cannot take an error.
// A missing setting is an error: a title whose flag is on never quietly keeps local tables.
// The title is registered in both services now; a failure there is logged and retried by the
// pending-row loop, because the first write registers it as well.
func NewCoreRankings(cfg Config, t CoreTitle, db *sql.DB) (*coreranking.Store, error) {
	lbConn, err := coreConn("NEX_LEADERBOARDS_ADDR", "NEX_LEADERBOARDS_KEY", "LEADERBOARDS_INTERNAL_KEY")
	if err != nil {
		return nil, err
	}
	tsConn, err := coreConn("NEX_TITLE_STORAGE_ADDR", "NEX_TITLE_STORAGE_KEY", "TITLE_STORAGE_INTERNAL_KEY")
	if err != nil {
		return nil, err
	}
	if cfg.AccountGRPCAddr == "" {
		return nil, fmt.Errorf("core rankings: NEX_ACCOUNT_GRPC_ADDR is required (PID to account)")
	}
	res, err := coreid.DialResolution(cfg.AccountGRPCAddr, cfg.AccountGRPCKey, t.Network)
	if err != nil {
		return nil, err
	}
	s := &coreranking.Store{
		Cfg: coreranking.Config{Title: t.Key, Name: t.Name, Network: t.Network, Kind: t.Kind, Order: t.Order},
		LB:  lbpb.NewLeaderboardsClient(lbConn),
		TS:  tspb.NewTitleStorageClient(tsConn),
		IDs: &coreid.Cache{L: res},
		DB:  db,
	}
	if err := s.InitTables(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Register(ctx); err != nil {
		log.Printf("[%s] core rankings: register: %v (retried with the first write)", t.Key, err)
	} else {
		log.Printf("[%s] core rankings on: leaderboards + title-storage, network %s", t.Key, t.Network)
	}
	s.StartDrain()
	return s, nil
}

// coreConn dials a core gRPC service whose key travels as x-internal-key.
func coreConn(addrVar, keyVar, sharedKeyVar string) (*grpc.ClientConn, error) {
	addr, key := os.Getenv(addrVar), env(keyVar, os.Getenv(sharedKeyVar))
	if addr == "" || key == "" {
		return nil, fmt.Errorf("core storage: %s and %s (or %s) are required", addrVar, keyVar, sharedKeyVar)
	}
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20)),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, inv grpc.UnaryInvoker, o ...grpc.CallOption) error {
			return inv(metadata.AppendToOutgoingContext(ctx, "x-internal-key", key), method, req, reply, cc, o...)
		}))
}
