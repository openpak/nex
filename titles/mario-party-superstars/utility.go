package mps

// Utility (0x6E) settings.
//
// GetIntegerSettings takes one index and answers Map<UInt16, Int32>; GetStringSettings is
// the same shape with strings. The game calls GetIntegerSettings immediately after
// SecureConnection::Register, and a malformed answer there drops the connection about a
// second later — which is what happens when a server misreads the single index parameter as
// a list length and replies with a body the client cannot parse. nex-protocols-common-go
// reads it correctly; all that is left for us is what the map should contain.
//
// We do not know. No capture of this title's settings exists and no public reference server
// implements it, so the default is a correctly shaped empty map for every index — right on
// the wire, possibly incomplete in content. MPS_INTEGER_SETTINGS fills it in from a capture
// without a rebuild, because a wrong guess should cost a restart, not a release.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
)

// configuredIntegerSettings is MPS_INTEGER_SETTINGS, parsed once at startup.
var configuredIntegerSettings = parseIntegerSettings(os.Getenv("MPS_INTEGER_SETTINGS"))

func integerSettings(_ *common_globals.UtilityManager, _ types.PID, index types.UInt32) (types.Map[types.UInt16, types.UInt32], *nex.Error) {
	settings := configuredIntegerSettings[uint32(index)]
	if settings == nil {
		settings = types.NewMap[types.UInt16, types.UInt32]()
	}
	return settings, nil
}

// stringSettings is always empty: nothing has ever recorded a string setting for this title,
// and an empty map of the right shape is the only honest answer.
func stringSettings(_ *common_globals.UtilityManager, _ types.PID, _ types.UInt32) (types.Map[types.UInt16, types.String], *nex.Error) {
	return types.NewMap[types.UInt16, types.String](), nil
}

// parseIntegerSettings reads "index:key=value" entries separated by commas or whitespace,
// e.g. "0:0=60, 0:1=30, 10:0=1". A malformed entry is skipped with a warning rather than
// taking the server down, because these values are tuning knobs and not identity.
func parseIntegerSettings(spec string) map[uint32]types.Map[types.UInt16, types.UInt32] {
	settings := map[uint32]types.Map[types.UInt16, types.UInt32]{}

	for _, entry := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		index, key, value, err := parseIntegerSetting(entry)
		if err != nil {
			common_globals.Logger.Warningf("MPS_INTEGER_SETTINGS: %v", err)
			continue
		}
		if settings[index] == nil {
			settings[index] = types.NewMap[types.UInt16, types.UInt32]()
		}
		settings[index][types.NewUInt16(key)] = types.NewUInt32(value)
	}

	return settings
}

func parseIntegerSetting(entry string) (index uint32, key uint16, value uint32, err error) {
	target, rawValue, found := strings.Cut(entry, "=")
	if !found {
		return 0, 0, 0, fmt.Errorf("%q is not index:key=value", entry)
	}
	rawIndex, rawKey, found := strings.Cut(target, ":")
	if !found {
		return 0, 0, 0, fmt.Errorf("%q is not index:key=value", entry)
	}

	parsedIndex, err := strconv.ParseUint(strings.TrimSpace(rawIndex), 10, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%q: bad index: %w", entry, err)
	}
	parsedKey, err := strconv.ParseUint(strings.TrimSpace(rawKey), 10, 16)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%q: bad key: %w", entry, err)
	}
	// Signed on the wire (Map<UInt16, Sint32>), so accept a negative value and keep its
	// two's complement, which is the same 32 bits either way.
	parsedValue, err := strconv.ParseInt(strings.TrimSpace(rawValue), 10, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%q: bad value: %w", entry, err)
	}

	return uint32(parsedIndex), uint16(parsedKey), uint32(int32(parsedValue)), nil
}
