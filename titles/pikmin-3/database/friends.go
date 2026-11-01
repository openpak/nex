package database

import (
	"context"
	"os"
	"time"

	friends "github.com/PretendoNetwork/grpc/go/friends"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/openpak/nex/titles/pikmin-3/globals"
)

var friendsClient friends.FriendsClient

// friendPIDs asks nn-friends for a player's friends. It fails closed: an unreachable friend
// service means "no friends", so a friends ranking shows only the caller.
func friendPIDs(pid uint32) []uint32 {
	addr := os.Getenv("PN_PIKMIN3_FRIENDS_GRPC_ADDR")
	if addr == "" {
		return nil
	}
	if friendsClient == nil {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			globals.Logger.Error(err.Error())
			return nil
		}
		friendsClient = friends.NewFriendsClient(conn)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "X-API-Key", os.Getenv("PN_PIKMIN3_FRIENDS_GRPC_API_KEY"))
	res, err := friendsClient.GetUserFriendPIDs(ctx, &friends.GetUserFriendPIDsRequest{Pid: pid})
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil
	}
	return res.GetPids()
}
