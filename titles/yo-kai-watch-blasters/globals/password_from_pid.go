package globals

import (
	"context"
	nexhost "github.com/openpak/nex"

	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"

	pb "github.com/PretendoNetwork/grpc/go/account/v2"
	"google.golang.org/grpc/metadata"
)

func PasswordFromPID(pid *types.PID) (string, uint32) {
	ctx := metadata.NewOutgoingContext(context.Background(), common_globals.GRPCAccountCommonMetadata)

	response, err := common_globals.GRPCAccountClient.GetNEXPassword(ctx, &pb.GetNEXPasswordRequest{Pid: uint32(*pid)})
	if err != nil {
		Logger.Error(err.Error())
		return "", nexhost.NEXPasswordResult(err)
	}

	return response.Password, 0
}
