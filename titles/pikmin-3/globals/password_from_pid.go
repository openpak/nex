package globals

import (
	"context"
	nexhost "github.com/openpak/nex"

	"github.com/PretendoNetwork/nex-go/v2/types"

	pb "github.com/PretendoNetwork/grpc/go/account"
	"google.golang.org/grpc/metadata"
)

func PasswordFromPID(pid types.PID) (string, uint32) {
	ctx := metadata.NewOutgoingContext(context.Background(), GRPCAccountCommonMetadata)

	response, err := GRPCAccountClient.GetNEXPassword(ctx, &pb.GetNEXPasswordRequest{Pid: uint32(pid)})
	if err != nil {
		Logger.Error(err.Error())
		return "", nexhost.NEXPasswordResult(err)
	}

	return response.Password, 0
}
