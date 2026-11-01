package nex

// The Ranking (Splatoon) structures, from the NintendoClients wiki page "Ranking Protocol
// (Splatoon)". Field names follow the wiki; the fields it calls "Unknown" are kept as
// Unknown* and are what the Wii U game sends or expects but nobody has named.
//
// Upstream's open pull request #22 carries a first version of these; it extracts into a
// value receiver (so the request is never filled in), copies the wrong field in Copy and
// prints the wrong field in String, so they are rewritten here rather than ported.

import (
	"fmt"
	"reflect"

	"github.com/PretendoNetwork/nex-go/v2/types"
)

// structure writes the four content writers of a structure the same way: content into a
// copy of the stream, then the version header and the bytes.
func writeStructure(writable types.Writable, s types.Structure, fields ...types.RVType) {
	content := writable.CopyNew()
	for _, f := range fields {
		f.WriteTo(content)
	}
	b := content.Bytes()
	s.WriteHeaderTo(writable, uint32(len(b)))
	writable.Write(b)
}

func extractStructure(readable types.Readable, name string, s *types.Structure, fields ...types.RVTypePtr) error {
	if err := s.ExtractHeaderFrom(readable); err != nil {
		return fmt.Errorf("failed to extract %s header. %w", name, err)
	}
	for i, f := range fields {
		if err := f.ExtractFrom(readable); err != nil {
			return fmt.Errorf("failed to extract %s field %d. %w", name, i, err)
		}
	}
	return nil
}

// CompetitionRankingGetParam is the GetCompetitionRankingScore request.
type CompetitionRankingGetParam struct {
	types.Structure
	Unknown     types.UInt32
	ResultRange types.ResultRange
	FestivalIDs types.List[types.UInt32]
}

func NewCompetitionRankingGetParam() CompetitionRankingGetParam {
	return CompetitionRankingGetParam{
		Structure:   types.Structure{StructureVersion: 1},
		Unknown:     types.NewUInt32(0),
		ResultRange: types.NewResultRange(),
		FestivalIDs: types.NewList[types.UInt32](),
	}
}

func (p CompetitionRankingGetParam) WriteTo(w types.Writable) {
	writeStructure(w, p.Structure, p.Unknown, p.ResultRange, p.FestivalIDs)
}

func (p *CompetitionRankingGetParam) ExtractFrom(r types.Readable) error {
	return extractStructure(r, "CompetitionRankingGetParam", &p.Structure, &p.Unknown, &p.ResultRange, &p.FestivalIDs)
}

func (p CompetitionRankingGetParam) Copy() types.RVType {
	c := p
	c.FestivalIDs = p.FestivalIDs.Copy().(types.List[types.UInt32])
	return c
}
func (p CompetitionRankingGetParam) Equals(o types.RVType) bool { return reflect.DeepEqual(p, o) }
func (p CompetitionRankingGetParam) CopyRef() types.RVTypePtr {
	c := p.Copy().(CompetitionRankingGetParam)
	return &c
}
func (p *CompetitionRankingGetParam) Deref() types.RVType { return *p }

// CompetitionRankingScoreData is one player's row in a festival's score list.
//
// Unknown1 and Unknown2 are unnamed on the wiki. This server fills Unknown1 with the
// 1-based position in the list and Unknown2 with the player's team id; neither has been
// confirmed against a console (see docs/splatfest.md in the title).
type CompetitionRankingScoreData struct {
	types.Structure
	Unknown1 types.UInt32
	PID      types.PID
	Score    types.UInt32
	Modified types.DateTime
	Unknown2 types.UInt8
	AppData  types.QBuffer
}

func NewCompetitionRankingScoreData() CompetitionRankingScoreData {
	return CompetitionRankingScoreData{
		Structure: types.Structure{StructureVersion: 1},
		Unknown1:  types.NewUInt32(0),
		PID:       types.NewPID(0),
		Score:     types.NewUInt32(0),
		Modified:  types.NewDateTime(0),
		Unknown2:  types.NewUInt8(0),
		AppData:   types.NewQBuffer(nil),
	}
}

func (d CompetitionRankingScoreData) WriteTo(w types.Writable) {
	writeStructure(w, d.Structure, d.Unknown1, d.PID, d.Score, d.Modified, d.Unknown2, d.AppData)
}

func (d *CompetitionRankingScoreData) ExtractFrom(r types.Readable) error {
	return extractStructure(r, "CompetitionRankingScoreData", &d.Structure,
		&d.Unknown1, &d.PID, &d.Score, &d.Modified, &d.Unknown2, &d.AppData)
}

func (d CompetitionRankingScoreData) Copy() types.RVType {
	c := d
	c.AppData = d.AppData.Copy().(types.QBuffer)
	return c
}
func (d CompetitionRankingScoreData) Equals(o types.RVType) bool { return reflect.DeepEqual(d, o) }
func (d CompetitionRankingScoreData) CopyRef() types.RVTypePtr {
	c := d.Copy().(CompetitionRankingScoreData)
	return &c
}
func (d *CompetitionRankingScoreData) Deref() types.RVType { return *d }

// CompetitionRankingScoreInfo is one festival in the GetCompetitionRankingScore reply:
// the score list plus the two per-team tallies, in team order (Alpha, Bravo).
type CompetitionRankingScoreInfo struct {
	types.Structure
	FestID    types.UInt32
	ScoreData types.List[CompetitionRankingScoreData]
	Unknown   types.UInt32
	TeamWins  types.List[types.UInt32]
	TeamVotes types.List[types.UInt32]
}

func NewCompetitionRankingScoreInfo() CompetitionRankingScoreInfo {
	return CompetitionRankingScoreInfo{
		Structure: types.Structure{StructureVersion: 1},
		FestID:    types.NewUInt32(0),
		ScoreData: types.NewList[CompetitionRankingScoreData](),
		Unknown:   types.NewUInt32(0),
		TeamWins:  types.NewList[types.UInt32](),
		TeamVotes: types.NewList[types.UInt32](),
	}
}

func (i CompetitionRankingScoreInfo) WriteTo(w types.Writable) {
	writeStructure(w, i.Structure, i.FestID, i.ScoreData, i.Unknown, i.TeamWins, i.TeamVotes)
}

func (i *CompetitionRankingScoreInfo) ExtractFrom(r types.Readable) error {
	return extractStructure(r, "CompetitionRankingScoreInfo", &i.Structure,
		&i.FestID, &i.ScoreData, &i.Unknown, &i.TeamWins, &i.TeamVotes)
}

func (i CompetitionRankingScoreInfo) Copy() types.RVType {
	c := i
	c.ScoreData = i.ScoreData.Copy().(types.List[CompetitionRankingScoreData])
	c.TeamWins = i.TeamWins.Copy().(types.List[types.UInt32])
	c.TeamVotes = i.TeamVotes.Copy().(types.List[types.UInt32])
	return c
}
func (i CompetitionRankingScoreInfo) Equals(o types.RVType) bool { return reflect.DeepEqual(i, o) }
func (i CompetitionRankingScoreInfo) CopyRef() types.RVTypePtr {
	c := i.Copy().(CompetitionRankingScoreInfo)
	return &c
}
func (i *CompetitionRankingScoreInfo) Deref() types.RVType { return *i }

// CompetitionRankingUploadScoreParam is the UploadCompetitionRankingScore request: one
// finished Splatfest match from one player.
type CompetitionRankingUploadScoreParam struct {
	types.Structure
	Unknown1      types.UInt32
	FestID        types.UInt32
	Unknown2      types.UInt32
	Score         types.UInt32
	TeamID        types.UInt8
	TeamScore     types.UInt32
	IsFirstUpload types.Bool
	AppData       types.QBuffer
}

func NewCompetitionRankingUploadScoreParam() CompetitionRankingUploadScoreParam {
	return CompetitionRankingUploadScoreParam{
		Structure:     types.Structure{StructureVersion: 1},
		Unknown1:      types.NewUInt32(0),
		FestID:        types.NewUInt32(0),
		Unknown2:      types.NewUInt32(0),
		Score:         types.NewUInt32(0),
		TeamID:        types.NewUInt8(0),
		TeamScore:     types.NewUInt32(0),
		IsFirstUpload: types.NewBool(false),
		AppData:       types.NewQBuffer(nil),
	}
}

func (p CompetitionRankingUploadScoreParam) WriteTo(w types.Writable) {
	writeStructure(w, p.Structure, p.Unknown1, p.FestID, p.Unknown2, p.Score, p.TeamID, p.TeamScore, p.IsFirstUpload, p.AppData)
}

func (p *CompetitionRankingUploadScoreParam) ExtractFrom(r types.Readable) error {
	return extractStructure(r, "CompetitionRankingUploadScoreParam", &p.Structure,
		&p.Unknown1, &p.FestID, &p.Unknown2, &p.Score, &p.TeamID, &p.TeamScore, &p.IsFirstUpload, &p.AppData)
}

func (p CompetitionRankingUploadScoreParam) Copy() types.RVType {
	c := p
	c.AppData = p.AppData.Copy().(types.QBuffer)
	return c
}
func (p CompetitionRankingUploadScoreParam) Equals(o types.RVType) bool {
	return reflect.DeepEqual(p, o)
}
func (p CompetitionRankingUploadScoreParam) CopyRef() types.RVTypePtr {
	c := p.Copy().(CompetitionRankingUploadScoreParam)
	return &c
}
func (p *CompetitionRankingUploadScoreParam) Deref() types.RVType { return *p }
