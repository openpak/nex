package local_matchmake_extension_database

import (
	"database/sql"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmaking_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	pqextended "github.com/PretendoNetwork/pq-extended"
)

// FindMatchmakeSessionByParticipant finds a MatchmakeSession on a database using the FindMatchmakeSessionByParticipantParam. Returns a list of FindMatchmakeSessionByParticipantResults
func FindMatchmakeSessionByParticipant(manager *common_globals.MatchmakingManager, connection *nex.PRUDPConnection, param matchmaking_types.FindMatchmakeSessionByParticipantParam) (types.List[matchmaking_types.FindMatchmakeSessionByParticipantResult], *nex.Error) {
	// TODO: find out what resultOptions is for

	endpoint := connection.Endpoint().(*nex.PRUDPEndPoint)

	rows, err := manager.Database.Query(`SELECT
		g.id,
		g.owner_pid,
		g.host_pid,
		g.min_participants,
		g.max_participants,
		g.participation_policy,
		g.policy_argument,
		g.flags,
		g.state,
		g.description,
		array_length(g.participants, 1),
		g.started_time,
		ms.game_mode,
		ms.attribs,
		ms.open_participation,
		ms.matchmake_system_type,
		ms.application_buffer,
		ms.progress_score,
		ms.session_key,
		ms.option_zero,
		ms.matchmake_param,
		ms.user_password,
		ms.refer_gid,
		ms.user_password_enabled,
		ms.system_password_enabled,
		ms.codeword
		FROM matchmaking.gatherings AS g
		INNER JOIN matchmaking.matchmake_sessions AS ms ON ms.id = g.id
		WHERE
		g.registered=true AND
		g.type='MatchmakeSession' AND
		g.host_pid <> 0 AND
		g.owner_pid <> 0 AND
		g.participants && $1 AND
		ms.open_participation=true AND
		array_length(g.participants, 1) < g.max_participants AND
		ms.user_password_enabled=false AND
		ms.system_password_enabled=false
		LIMIT 50`,
		pqextended.Array(param.PrincipalIDList),
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return types.NewList[matchmaking_types.FindMatchmakeSessionByParticipantResult](), nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, err.Error())
		} else {
			return types.NewList[matchmaking_types.FindMatchmakeSessionByParticipantResult](), nex.NewError(nex.ResultCodes.Core.Unknown, err.Error())
		}
	}

	findByParticipantResults := types.NewList[matchmaking_types.FindMatchmakeSessionByParticipantResult]()

	for rows.Next() {
		findByParticipantResult := matchmaking_types.NewFindMatchmakeSessionByParticipantResult()
		var startedTime time.Time
		var resultAttribs []uint32
		var resultMatchmakeParam []byte

		err = rows.Scan(
			&findByParticipantResult.Session.Gathering.ID,
			&findByParticipantResult.Session.Gathering.OwnerPID,
			&findByParticipantResult.Session.Gathering.HostPID,
			&findByParticipantResult.Session.Gathering.MinimumParticipants,
			&findByParticipantResult.Session.Gathering.MaximumParticipants,
			&findByParticipantResult.Session.Gathering.ParticipationPolicy,
			&findByParticipantResult.Session.Gathering.PolicyArgument,
			&findByParticipantResult.Session.Gathering.Flags,
			&findByParticipantResult.Session.Gathering.State,
			&findByParticipantResult.Session.Gathering.Description,
			&findByParticipantResult.Session.ParticipationCount,
			&startedTime,
			&findByParticipantResult.Session.GameMode,
			pqextended.Array(&resultAttribs),
			&findByParticipantResult.Session.OpenParticipation,
			&findByParticipantResult.Session.MatchmakeSystemType,
			&findByParticipantResult.Session.ApplicationBuffer,
			&findByParticipantResult.Session.ProgressScore,
			&findByParticipantResult.Session.SessionKey,
			&findByParticipantResult.Session.Option,
			&resultMatchmakeParam,
			&findByParticipantResult.Session.UserPassword,
			&findByParticipantResult.Session.ReferGID,
			&findByParticipantResult.Session.UserPasswordEnabled,
			&findByParticipantResult.Session.SystemPasswordEnabled,
			&findByParticipantResult.Session.CodeWord,
		)

		if err != nil {
			common_globals.Logger.Critical(err.Error())
			continue
		}

		findByParticipantResult.Session.StartedTime = findByParticipantResult.Session.StartedTime.FromTimestamp(startedTime)

		attributesSlice := make([]types.UInt32, len(resultAttribs))
		for i, value := range resultAttribs {
			attributesSlice[i] = types.NewUInt32(value)
		}
		findByParticipantResult.Session.Attributes = attributesSlice

		matchmakeParamBytes := nex.NewByteStreamIn(resultMatchmakeParam, endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
		findByParticipantResult.Session.MatchmakeParam.ExtractFrom(matchmakeParamBytes)

		findByParticipantResults = append(findByParticipantResults, findByParticipantResult)
	}

	rows.Close()

	return findByParticipantResults, nil
}
