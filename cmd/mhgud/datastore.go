package main

// DataStore (protocol 0x73) backed by Postgres for the metadata and an S3-compatible store
// for the object bodies.
//
// nex-protocols-common-go already implements every DataStore method the game can call. What
// it leaves open is where the rows live, as a set of callbacks; this file is those
// callbacks and the two tables behind them. Nothing here is MHGU-specific, because MHGU's
// own use of DataStore is still unmeasured — the game links the module, and that is all we
// know. A generic store is the honest answer to that: the first object a console posts will
// say in its own row what the title actually does with it.

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_datastore "github.com/PretendoNetwork/nex-protocols-common-go/v2/datastore"
	"github.com/PretendoNetwork/nex-protocols-go/v2/datastore/constants"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
)

// searchLimit caps how many objects one SearchObject call may return.
// ponytail: a fixed cap, not a paging cursor. The client asks for a range and gets at most
// this many; raise it if a capture ever shows MHGU asking for more in one call.
const searchLimit = 100

const schemaSQL = `
CREATE SCHEMA IF NOT EXISTS datastore;

CREATE TABLE IF NOT EXISTS datastore.objects (
	data_id          bigserial PRIMARY KEY,
	owner_pid        numeric(20) NOT NULL,
	size             bigint      NOT NULL DEFAULT 0,
	name             text        NOT NULL DEFAULT '',
	data_type        integer     NOT NULL DEFAULT 0,
	meta_binary      bytea       NOT NULL DEFAULT '',
	permission       integer     NOT NULL DEFAULT 0,
	recipients       numeric(20)[] NOT NULL DEFAULT '{}',
	del_permission   integer     NOT NULL DEFAULT 3,
	del_recipients   numeric(20)[] NOT NULL DEFAULT '{}',
	period           integer     NOT NULL DEFAULT 0,
	refer_data_id    bigint      NOT NULL DEFAULT 0,
	flag             integer     NOT NULL DEFAULT 0,
	tags             text[]      NOT NULL DEFAULT '{}',
	access_password  numeric(20) NOT NULL DEFAULT 0,
	update_password  numeric(20) NOT NULL DEFAULT 0,
	persistence_slot integer,
	uploaded         boolean     NOT NULL DEFAULT false,
	created_at       timestamptz NOT NULL DEFAULT now(),
	updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS objects_persistence_slot
	ON datastore.objects (owner_pid, persistence_slot)
	WHERE persistence_slot IS NOT NULL;

CREATE TABLE IF NOT EXISTS datastore.ratings (
	data_id   bigint   NOT NULL REFERENCES datastore.objects (data_id) ON DELETE CASCADE,
	slot      smallint NOT NULL,
	total     bigint   NOT NULL DEFAULT 0,
	count     integer  NOT NULL DEFAULT 0,
	initial   bigint   NOT NULL DEFAULT 0,
	range_min integer  NOT NULL DEFAULT 0,
	range_max integer  NOT NULL DEFAULT 0,
	PRIMARY KEY (data_id, slot)
);
`

// metaColumns is the projection scanMeta expects. The two array columns are cast to text
// because nex-go's List type scans a PostgreSQL array literal, not a driver array.
const metaColumns = `data_id, owner_pid, size, name, data_type, meta_binary, permission,
	recipients::text, del_permission, del_recipients::text, period, refer_data_id, flag,
	tags::text, created_at, updated_at`

type store struct{ db *sql.DB }

func newStore(db *sql.DB) (*store, error) {
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

// wire attaches the store to the common DataStore protocol. Every callback the common
// implementation checks for is set here; the ones it does not have, it answers
// Core::NotImplemented to on its own.
func (s *store) wire(common *common_datastore.CommonProtocol, friendPIDs func(uint32) []uint32) {
	common.GetUserFriendPIDs = friendPIDs
	common.GetObjectInfoByDataID = s.meta
	common.GetObjectInfoByDataIDWithPassword = s.metaWithPassword
	common.GetObjectInfoByPersistenceTargetWithPassword = s.metaByPersistence
	common.GetObjectSizeByDataID = s.size
	common.GetObjectOwnerByDataID = s.owner
	common.UpdateObjectUploadCompletedByDataID = s.setUploaded
	common.UpdateObjectPeriodByDataIDWithPassword = s.setPeriod
	common.UpdateObjectMetaBinaryByDataIDWithPassword = s.setMetaBinary
	common.UpdateObjectDataTypeByDataIDWithPassword = s.setDataType
	common.InitializeObjectByPreparePostParam = s.create
	common.InitializeObjectRatingWithSlot = s.createRating
	common.RateObjectWithPassword = s.rate
	common.DeleteObjectByDataID = s.delete
	common.DeleteObjectByDataIDWithPassword = s.deleteWithPassword
	common.GetObjectInfosByDataStoreSearchParam = s.search
}

// meta answers only for objects whose upload has completed. CompletePostObject depends on
// that: it treats a row it can read here as one that was already finished.
func (s *store) meta(dataID types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	row := s.db.QueryRow(`SELECT `+metaColumns+` FROM datastore.objects WHERE data_id = $1 AND uploaded`, uint64(dataID))
	return s.scanOne(row)
}

func (s *store) metaWithPassword(dataID types.UInt64, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	row := s.db.QueryRow(`SELECT `+metaColumns+` FROM datastore.objects
		WHERE data_id = $1 AND uploaded AND (access_password = 0 OR access_password = $2)`,
		uint64(dataID), fmt.Sprint(uint64(password)))
	return s.scanOne(row)
}

func (s *store) metaByPersistence(target datastore_types.DataStorePersistenceTarget, password types.UInt64) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	row := s.db.QueryRow(`SELECT `+metaColumns+` FROM datastore.objects
		WHERE owner_pid = $1 AND persistence_slot = $2 AND uploaded
		  AND (access_password = 0 OR access_password = $3)`,
		fmt.Sprint(uint64(target.OwnerID)), uint16(target.PersistenceSlotID), fmt.Sprint(uint64(password)))
	return s.scanOne(row)
}

func (s *store) size(dataID types.UInt64) (uint32, *nex.Error) {
	var size uint32
	err := s.db.QueryRow(`SELECT size FROM datastore.objects WHERE data_id = $1`, uint64(dataID)).Scan(&size)
	if err != nil {
		return 0, notFound(err)
	}
	return size, nil
}

func (s *store) owner(dataID types.UInt64) (uint32, *nex.Error) {
	var owner uint64
	err := s.db.QueryRow(`SELECT owner_pid FROM datastore.objects WHERE data_id = $1`, uint64(dataID)).Scan(&owner)
	if err != nil {
		return 0, notFound(err)
	}
	return uint32(owner), nil
}

func (s *store) setUploaded(dataID types.UInt64, uploaded bool) *nex.Error {
	return s.exec(`UPDATE datastore.objects SET uploaded = $2, updated_at = now() WHERE data_id = $1`,
		uint64(dataID), uploaded)
}

func (s *store) setPeriod(dataID types.UInt64, period types.UInt16, password types.UInt64) *nex.Error {
	return s.exec(`UPDATE datastore.objects SET period = $2, updated_at = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`,
		uint64(dataID), uint16(period), fmt.Sprint(uint64(password)))
}

func (s *store) setMetaBinary(dataID types.UInt64, metaBinary types.QBuffer, password types.UInt64) *nex.Error {
	return s.exec(`UPDATE datastore.objects SET meta_binary = $2, updated_at = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`,
		uint64(dataID), []byte(metaBinary), fmt.Sprint(uint64(password)))
}

func (s *store) setDataType(dataID types.UInt64, dataType types.UInt16, password types.UInt64) *nex.Error {
	return s.exec(`UPDATE datastore.objects SET data_type = $2, updated_at = now()
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $3)`,
		uint64(dataID), uint16(dataType), fmt.Sprint(uint64(password)))
}

// create reserves a data ID for an object the client is about to upload. The row is not
// visible to meta lookups until CompletePostObject marks it uploaded.
func (s *store) create(ownerPID types.PID, param datastore_types.DataStorePreparePostParam) (uint64, *nex.Error) {
	slot := any(nil)
	if id := uint16(param.PersistenceInitParam.PersistenceSlotID); id != noPersistenceSlot {
		slot = int(id)
		// The slot holds one object per owner. Whatever was in it stops being the
		// persistent object; the row itself stays, reachable by its data ID.
		// ponytail: we always displace, and ignore DeleteLastObject — nex-protocols-common-go
		// carries its own TODO saying what that flag means is not established.
		// The first object in a slot displaces nothing, so this is one of the few
		// statements here allowed to affect no rows.
		if _, err := s.db.Exec(`UPDATE datastore.objects SET persistence_slot = NULL
			WHERE owner_pid = $1 AND persistence_slot = $2`, fmt.Sprint(uint64(ownerPID)), int(id)); err != nil {
			return 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
		}
	}

	var dataID uint64
	err := s.db.QueryRow(`INSERT INTO datastore.objects
		(owner_pid, size, name, data_type, meta_binary, permission, recipients, del_permission,
		 del_recipients, period, refer_data_id, flag, tags, persistence_slot)
		VALUES ($1, $2, $3, $4, $5, $6, $7::numeric(20)[], $8, $9::numeric(20)[], $10, $11, $12, $13::text[], $14)
		RETURNING data_id`,
		fmt.Sprint(uint64(ownerPID)),
		uint32(param.Size),
		string(param.Name),
		uint16(param.DataType),
		[]byte(param.MetaBinary),
		uint8(param.Permission.Permission),
		arrayLiteral(param.Permission.RecipientIDs),
		uint8(param.DelPermission.Permission),
		arrayLiteral(param.DelPermission.RecipientIDs),
		uint16(param.Period),
		uint32(param.ReferDataID),
		uint8(param.Flag),
		arrayLiteral(param.Tags),
		slot,
	).Scan(&dataID)
	if err != nil {
		return 0, nex.NewError(nex.ResultCodes.DataStore.OperationNotAllowed, err.Error())
	}
	return dataID, nil
}

func (s *store) createRating(dataID uint64, param datastore_types.DataStoreRatingInitParamWithSlot) *nex.Error {
	return s.exec(`INSERT INTO datastore.ratings (data_id, slot, total, count, initial, range_min, range_max)
		VALUES ($1, $2, $3, 0, $3, $4, $5)
		ON CONFLICT (data_id, slot) DO NOTHING`,
		dataID, int16(param.Slot), int64(param.Param.InitialValue),
		int32(param.Param.RangeMin), int32(param.Param.RangeMax))
}

// rate adds one vote to a slot and returns the slot's new totals.
// ponytail: no per-user vote log, so nothing stops the same account rating twice. Add a
// (data_id, slot, pid) table if a title ever exposes ratings to players.
func (s *store) rate(dataID types.UInt64, slot types.UInt8, value types.Int32, password types.UInt64) (datastore_types.DataStoreRatingInfo, *nex.Error) {
	rating := datastore_types.NewDataStoreRatingInfo()
	row := s.db.QueryRow(`UPDATE datastore.ratings AS r SET total = r.total + $3, count = r.count + 1
		FROM datastore.objects AS o
		WHERE o.data_id = r.data_id AND r.data_id = $1 AND r.slot = $2
		  AND (o.access_password = 0 OR o.access_password = $4)
		  AND ($3 BETWEEN r.range_min AND r.range_max OR r.range_min = r.range_max)
		RETURNING r.total, r.count, r.initial`,
		uint64(dataID), uint8(slot), int32(value), fmt.Sprint(uint64(password)))

	var total, initial int64
	var count uint32
	if err := row.Scan(&total, &count, &initial); err != nil {
		return rating, notFound(err)
	}
	rating.TotalValue = types.NewInt64(total)
	rating.Count = types.NewUInt32(count)
	rating.InitialValue = types.NewInt64(initial)
	return rating, nil
}

func (s *store) delete(dataID types.UInt64) *nex.Error {
	return s.exec(`DELETE FROM datastore.objects WHERE data_id = $1`, uint64(dataID))
}

func (s *store) deleteWithPassword(dataID types.UInt64, password types.UInt64) *nex.Error {
	return s.exec(`DELETE FROM datastore.objects
		WHERE data_id = $1 AND (update_password = 0 OR update_password = $2)`,
		uint64(dataID), fmt.Sprint(uint64(password)))
}

func (s *store) search(param datastore_types.DataStoreSearchParam, _ types.PID) ([]datastore_types.DataStoreMetaInfo, uint32, *nex.Error) {
	where, order, args, limit, offset := searchQuery(param)

	var total uint32
	if err := s.db.QueryRow(`SELECT count(*) FROM datastore.objects WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM datastore.objects WHERE %s ORDER BY %s LIMIT %d OFFSET %d`,
		metaColumns, where, order, limit, offset), args...)
	if err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	defer rows.Close()

	var objects []datastore_types.DataStoreMetaInfo
	for rows.Next() {
		object, nexErr := s.scanOne(rows)
		if nexErr != nil {
			return nil, 0, nexErr
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	return objects, total, nil
}

// searchQuery turns a search parameter into a WHERE clause, an ORDER BY, its bind arguments
// and the range to return. Split out from search so it can be checked without a database.
//
// A zero DateTime, a zero ReferDataID and an empty list all mean "not filtering on this".
// DataType 0xFFFF means the same — see docs/design.md, it is an assumption a capture has
// yet to confirm.
func searchQuery(param datastore_types.DataStoreSearchParam) (where, order string, args []any, limit, offset uint32) {
	conditions := []string{"uploaded"}
	add := func(format string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(format, len(args)))
	}

	if len(param.OwnerIDs) > 0 {
		add("owner_pid = ANY($%d::numeric(20)[])", arrayLiteral(param.OwnerIDs))
	}
	if len(param.DataTypes) > 0 {
		add("data_type = ANY($%d::integer[])", arrayLiteral(param.DataTypes))
	} else if uint16(param.DataType) != anyDataType {
		add("data_type = $%d", uint16(param.DataType))
	}
	if len(param.Tags) > 0 {
		add("tags @> $%d::text[]", arrayLiteral(param.Tags))
	}
	if uint32(param.ReferDataID) != 0 {
		add("refer_data_id = $%d", uint32(param.ReferDataID))
	}
	for _, filter := range []struct {
		format string
		value  types.DateTime
	}{
		{"created_at >= $%d", param.CreatedAfter},
		{"created_at <= $%d", param.CreatedBefore},
		{"updated_at >= $%d", param.UpdatedAfter},
		{"updated_at <= $%d", param.UpdatedBefore},
	} {
		if uint64(filter.value) != 0 {
			add(filter.format, filter.value.Standard())
		}
	}

	limit = uint32(param.ResultRange.Length)
	if limit == 0 || limit > searchLimit {
		limit = searchLimit
	}
	return strings.Join(conditions, " AND "), sortOrder(param), args, limit, uint32(param.ResultRange.Offset)
}

// anyDataType is the data type value that means "do not filter by data type".
const anyDataType = 0xFFFF

// noPersistenceSlot is the persistence slot that means "this object is not persistent".
const noPersistenceSlot = 0xFFFF

func sortOrder(param datastore_types.DataStoreSearchParam) string {
	column := "data_id"
	switch param.ResultOrderColumn {
	case constants.SearchSortColumnSize:
		column = "size"
	case constants.SearchSortColumnNameAlphabetical:
		column = "name"
	case constants.SearchSortColumnDataType:
		column = "data_type"
	case constants.SearchSortColumnCreatedTime:
		column = "created_at"
	case constants.SearchSortColumnUpdatedTime:
		column = "updated_at"
	}
	if param.ResultOrder == constants.SearchSortOrderDesc {
		return column + " DESC"
	}
	return column + " ASC"
}

// row is what scanOne reads: sql.Row and sql.Rows both satisfy it.
type row interface {
	Scan(dest ...any) error
}

func (s *store) scanOne(source row) (datastore_types.DataStoreMetaInfo, *nex.Error) {
	meta := datastore_types.NewDataStoreMetaInfo()

	var (
		dataID, ownerPID          uint64
		size, referDataID         uint32
		name                      string
		dataType, period          uint16
		permission, delPermission uint8
		flag                      uint8
		metaBinary                []byte
		createdAt, updatedAt      time.Time
	)
	err := source.Scan(&dataID, &ownerPID, &size, &name, &dataType, &metaBinary, &permission,
		&meta.Permission.RecipientIDs, &delPermission, &meta.DelPermission.RecipientIDs,
		&period, &referDataID, &flag, &meta.Tags, &createdAt, &updatedAt)
	if err != nil {
		return meta, notFound(err)
	}

	meta.DataID = types.NewUInt64(dataID)
	meta.OwnerID = types.NewPID(ownerPID)
	meta.Size = types.NewUInt32(size)
	meta.Name = types.NewString(name)
	meta.DataType = types.NewUInt16(dataType)
	meta.MetaBinary = types.NewQBuffer(metaBinary)
	meta.Permission.Permission = constants.Permission(permission)
	meta.DelPermission.Permission = constants.Permission(delPermission)
	meta.CreatedTime = dateTime(createdAt)
	meta.UpdatedTime = dateTime(updatedAt)
	meta.Period = types.NewUInt16(period)
	meta.Status = constants.DataStatusNone
	meta.ReferDataID = types.NewUInt32(referDataID)
	meta.Flag = constants.DataFlag(flag)
	meta.ReferredTime = dateTime(updatedAt)
	// ponytail: expiry is a number the client is told, not a sweep we run. Nothing deletes
	// an expired object; add a periodic DELETE if a bucket ever fills up.
	meta.ExpireTime = dateTime(expiry(createdAt, period))

	ratings, nexErr := s.ratings(dataID)
	if nexErr != nil {
		return meta, nexErr
	}
	meta.Ratings = ratings
	return meta, nil
}

// ratings reads an object's rating slots.
// ponytail: one query per object, so a search of N objects costs N+1 queries. Join it if a
// search ever returns enough rows for that to matter.
func (s *store) ratings(dataID uint64) (types.List[datastore_types.DataStoreRatingInfoWithSlot], *nex.Error) {
	list := types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()

	rows, err := s.db.Query(`SELECT slot, total, count, initial FROM datastore.ratings WHERE data_id = $1 ORDER BY slot`, dataID)
	if err != nil {
		return list, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var slot int16
		var total, initial int64
		var count uint32
		if err := rows.Scan(&slot, &total, &count, &initial); err != nil {
			return list, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
		}
		entry := datastore_types.NewDataStoreRatingInfoWithSlot()
		entry.Slot = types.NewInt8(int8(slot))
		entry.Rating.TotalValue = types.NewInt64(total)
		entry.Rating.Count = types.NewUInt32(count)
		entry.Rating.InitialValue = types.NewInt64(initial)
		list = append(list, entry)
	}
	return list, nil
}

// exec runs a statement that must change exactly one row; nothing changed means the object
// is not there or the password did not match, and the client is told the same either way.
func (s *store) exec(query string, args ...any) *nex.Error {
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object, or the password did not match")
	}
	return nil
}

func notFound(err error) *nex.Error {
	if errors.Is(err, sql.ErrNoRows) {
		return nex.NewError(nex.ResultCodes.DataStore.NotFound, "no such object")
	}
	return nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
}

// expiry is when an object stops being served. A period of zero means the object does not
// expire, which the wire has no way to say, so it is given a date far enough out to mean it.
func expiry(created time.Time, period uint16) time.Time {
	if period == 0 {
		return created.AddDate(100, 0, 0)
	}
	return created.AddDate(0, 0, int(period))
}

func dateTime(t time.Time) types.DateTime {
	value := types.NewDateTime(0)
	return value.FromTimestamp(t.UTC())
}

// arrayLiteral renders a NEX list as the PostgreSQL array literal its own Value method
// produces, so it can be bound as text and cast in the statement.
func arrayLiteral(list interface{ Value() (driver.Value, error) }) any {
	value, err := list.Value()
	if err != nil {
		return "{}"
	}
	return value
}
