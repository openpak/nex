package nex_matchmaking_database

import (
	"database/sql"

	"github.com/PretendoNetwork/monster-hunter-3-ultimate/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmaking_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
)

// FindPersistentGatheringsByOwner finds persistent gatherings from a database using the given owner's pid. Returns a list of GatheringHolders
func FindPersistentGatheringsByOwner(manager *common_globals.MatchmakingManager, connection *nex.PRUDPConnection, pid types.PID, userpid types.PID, resultRange types.ResultRange) (types.List[matchmaking_types.GatheringHolder], *nex.Error) {
	persistentGatherings := types.NewList[matchmaking_types.GatheringHolder]()

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
		pg.community_type,
		pg.password,
		pg.attribs,
		pg.application_buffer,
		pg.participation_start_date,
		pg.participation_end_date,
		(SELECT COUNT(ms.id)
			FROM matchmaking.matchmake_sessions AS ms
			INNER JOIN matchmaking.gatherings AS gms ON ms.id = gms.id
			WHERE gms.registered=true
			AND ms.matchmake_system_type=5 -- matchmake_system_type=5 is only used in matchmake sessions attached to a persistent gathering
			AND ms.attribs[1]=g.id) AS matchmake_session_count,
		array_length(g.participants, 1)
		FROM matchmaking.gatherings AS g
		INNER JOIN matchmaking.persistent_gatherings AS pg ON g.id = pg.id
		WHERE
		g.registered=true AND
		g.type='PersistentGathering' AND
		g.owner_pid = $1
		ORDER BY id ASC`,
		pid,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			globals.Logger.Error(err.Error())
			return types.NewList[matchmaking_types.GatheringHolder](), nex.NewError(nex.ResultCodes.RendezVous.SessionVoid, err.Error())
		} else {
			globals.Logger.Error(err.Error())
			return types.NewList[matchmaking_types.GatheringHolder](), nex.NewError(nex.ResultCodes.Core.Unknown, err.Error())
		}
	}

	for rows.Next() {
		resultGatheringHolder := matchmaking_types.NewGatheringHolder()
		persistentGathering := matchmaking_types.NewPersistentGathering()
		err = rows.Scan(
			&persistentGathering.Gathering.ID,
			&persistentGathering.Gathering.OwnerPID,
			&persistentGathering.Gathering.HostPID,
			&persistentGathering.Gathering.MinimumParticipants,
			&persistentGathering.Gathering.MaximumParticipants,
			&persistentGathering.Gathering.ParticipationPolicy,
			&persistentGathering.Gathering.PolicyArgument,
			&persistentGathering.Gathering.Flags,
			&persistentGathering.Gathering.State,
			&persistentGathering.Gathering.Description,
			&persistentGathering.CommunityType,
			&persistentGathering.Password,
			&persistentGathering.Attribs,
			&persistentGathering.ApplicationBuffer,
			&persistentGathering.ParticipationStartDate,
			&persistentGathering.ParticipationEndDate,
			&persistentGathering.MatchmakeSessionCount,
			&persistentGathering.ParticipationCount,
		)
		if err != nil {
			common_globals.Logger.Critical(err.Error())
			continue
		}

		resultGatheringHolder.Object = persistentGathering.Copy().(matchmaking_types.GatheringInterface)
		persistentGatherings = append(persistentGatherings, resultGatheringHolder)
	}

	rows.Close()

	return persistentGatherings, nil
}
