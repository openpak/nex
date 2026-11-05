package coredatastore

import (
	"context"
	"os"
	"strconv"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/openpak/nex/internal/coreid"
	tspb "github.com/openpak/nex/internal/titlestoragepb"
)

// TestLiveRoundTrip runs the round trip against a real title-storage and nn-account (bodies over
// the public bucket host, as a console sends them). Off unless NEX_LIVE_TITLE_STORAGE_ADDR is set:
//
//	NEX_LIVE_TITLE_STORAGE_ADDR, TITLE_STORAGE_INTERNAL_KEY   title-storage gRPC and its key
//	NEX_ACCOUNT_GRPC_ADDR, NEX_ACCOUNT_GRPC_KEY                nn-account (PID <-> account)
//	NEX_LIVE_TITLE (default mario-kart-8), NEX_LIVE_PID, NEX_LIVE_PID2 (default NEX_LIVE_PID)
//
// It writes one throwaway object (data type 65000) and deletes it.
func TestLiveRoundTrip(t *testing.T) {
	addr := os.Getenv("NEX_LIVE_TITLE_STORAGE_ADDR")
	if addr == "" {
		t.Skip("NEX_LIVE_TITLE_STORAGE_ADDR not set")
	}
	key := os.Getenv("TITLE_STORAGE_INTERNAL_KEY")
	owner, err := strconv.ParseUint(os.Getenv("NEX_LIVE_PID"), 10, 64)
	if err != nil || key == "" {
		t.Fatal("NEX_LIVE_PID and TITLE_STORAGE_INTERNAL_KEY are required")
	}
	other := owner
	if v := os.Getenv("NEX_LIVE_PID2"); v != "" {
		if other, err = strconv.ParseUint(v, 10, 64); err != nil {
			t.Fatal(err)
		}
	}
	title := os.Getenv("NEX_LIVE_TITLE")
	if title == "" {
		title = "mario-kart-8"
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, m string, req, reply any, cc *grpc.ClientConn, inv grpc.UnaryInvoker, o ...grpc.CallOption) error {
			return inv(metadata.AppendToOutgoingContext(ctx, "x-internal-key", key), m, req, reply, cc, o...)
		}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := coreid.DialResolution(os.Getenv("NEX_ACCOUNT_GRPC_ADDR"), os.Getenv("NEX_ACCOUNT_GRPC_KEY"), "wiiu")
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Cfg: Config{Title: title, Network: "wiiu"}, TS: tspb.NewTitleStorageClient(conn), IDs: &coreid.Cache{L: res}}
	roundTrip(t, s, owner, other, 65000)
}
