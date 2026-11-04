package nexhost

// The database half of account erasure. It is driven by the schema the container's database
// actually has (information_schema), not by a list of titles: forty titles share a handful of table
// shapes (Pretendo's DataStore, Ranking, Matchmaking, Messaging tables) and the ones that do not are
// caught the same way. Rules, per table:
//
//   - a scalar PID column (pid, *_pid, owner, principal_id, recipient_id): the row is the player's and
//     is deleted, children that reference it (foreign keys) first;
//   - an array of PIDs (participants, *recipients, *pids): the PID is removed from the array;
//   - a DataStore object table (primary key data_id): deleted rows' ids are returned so the bodies in
//     the bucket go too, and every other table of the schema keyed by data_id loses those rows;
//   - exceptions, kept and credited to nobody (PID 0) like published mods: tournaments a player
//     created (mk8.tournaments), the world-record columns on other people's courses
//     (datastore.course_records), and, for the two course-making titles, the player's published
//     courses (datastore.objects: public, completed, not deleted).
//
// Everything runs in one transaction per database; a second run finds nothing.

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/lib/pq"
)

// scrubResult is what one database gave up.
type scrubResult struct {
	Rows    int
	DataIDs []int64 // objects whose bodies must leave the bucket
}

type colInfo struct {
	name, dataType, udt string
}

type tableKey struct{ schema, name string }

func (t tableKey) sql() string {
	if t.schema == "public" || t.schema == "" {
		return pq.QuoteIdentifier(t.name)
	}
	return pq.QuoteIdentifier(t.schema) + "." + pq.QuoteIdentifier(t.name)
}

func (t tableKey) String() string { return t.schema + "." + t.name }

type tableInfo struct {
	key     tableKey
	cols    []colInfo
	pkData  bool // primary key is exactly data_id
	hasData bool
}

func (t *tableInfo) has(name string) bool {
	for _, c := range t.cols {
		if c.name == name {
			return true
		}
	}
	return false
}

var (
	pidColumn      = regexp.MustCompile(`^(.*_)?pid$|^owner$|^principal_id$|^recipient_id$`)
	pidArrayColumn = regexp.MustCompile(`^participants$|recipients$|^(.*_)?pids$`)
	numericTypes   = map[string]bool{"smallint": true, "integer": true, "bigint": true, "numeric": true}
	numericArrays  = map[string]bool{"_int2": true, "_int4": true, "_int8": true, "_numeric": true}
)

// anonymise lists tables whose rows stay and whose PID columns are set to 0 instead.
var anonymise = map[string][]string{
	"mk8.tournaments":          {"owner_pid"},
	"datastore.course_records": {"first_pid", "best_pid"},
}

// keepPublic names the titles whose published DataStore objects (courses) outlive their maker.
var keepPublic = map[string]bool{"super-mario-maker": true, "super-mario-maker-2": true}

const publicCourse = `permission = 0 AND upload_completed AND NOT deleted`

func (t *tableInfo) pidCols() []string {
	var out []string
	for _, c := range t.cols {
		if numericTypes[c.dataType] && pidColumn.MatchString(c.name) {
			out = append(out, c.name)
		}
	}
	return out
}

func (t *tableInfo) arrayCols() []colInfo {
	var out []colInfo
	for _, c := range t.cols {
		if c.dataType == "ARRAY" && numericArrays[c.udt] && pidArrayColumn.MatchString(c.name) {
			out = append(out, c)
		}
	}
	return out
}

// matchSQL is the condition "this row belongs to one of the PIDs ($1)" for the given columns.
func (t *tableInfo) matchSQL(cols []string) string {
	var parts []string
	for _, c := range cols {
		q := pq.QuoteIdentifier(c)
		cond := q + " = ANY($1::numeric[])"
		if c == "recipient_id" && t.has("recipient_type") {
			cond = "(recipient_type = 1 AND " + cond + ")"
		}
		parts = append(parts, cond)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func loadSchema(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[tableKey]*tableInfo, error) {
	rows, err := q.QueryContext(ctx, `
SELECT c.table_schema, c.table_name, c.column_name, c.data_type, c.udt_name
FROM information_schema.columns c
JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
WHERE t.table_type = 'BASE TABLE' AND c.table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY c.table_schema, c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, err
	}
	tables := map[tableKey]*tableInfo{}
	for rows.Next() {
		var s, tn string
		var c colInfo
		if err := rows.Scan(&s, &tn, &c.name, &c.dataType, &c.udt); err != nil {
			rows.Close()
			return nil, err
		}
		k := tableKey{s, tn}
		if tables[k] == nil {
			tables[k] = &tableInfo{key: k}
		}
		tables[k].cols = append(tables[k].cols, c)
		if c.name == "data_id" {
			tables[k].hasData = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Tables whose primary key is exactly (data_id): the DataStore object tables.
	pk, err := q.QueryContext(ctx, `
SELECT n.nspname, c.relname
FROM pg_index i
JOIN pg_class c ON c.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE i.indisprimary AND i.indnatts = 1
  AND (SELECT attname FROM pg_attribute WHERE attrelid = i.indrelid AND attnum = i.indkey[0]) = 'data_id'`)
	if err != nil {
		return nil, err
	}
	defer pk.Close()
	for pk.Next() {
		var s, tn string
		if err := pk.Scan(&s, &tn); err != nil {
			return nil, err
		}
		if t := tables[tableKey{s, tn}]; t != nil {
			t.pkData = true
		}
	}
	return tables, pk.Err()
}

// fk is one foreign key: child columns reference parent columns.
type fk struct {
	child, parent tableKey
	cc, pc        []string
}

func loadFKs(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[tableKey][]fk, error) {
	rows, err := q.QueryContext(ctx, `
SELECT cn.nspname, cc.relname, pn.nspname, pc.relname,
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(con.conkey) WITH ORDINALITY k(attnum, ord)
     JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum),
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(con.confkey) WITH ORDINALITY k(attnum, ord)
     JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.attnum)
FROM pg_constraint con
JOIN pg_class cc ON cc.oid = con.conrelid JOIN pg_namespace cn ON cn.oid = cc.relnamespace
JOIN pg_class pc ON pc.oid = con.confrelid JOIN pg_namespace pn ON pn.oid = pc.relnamespace
WHERE con.contype = 'f'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[tableKey][]fk{}
	for rows.Next() {
		var f fk
		var cc, pc pq.StringArray
		if err := rows.Scan(&f.child.schema, &f.child.name, &f.parent.schema, &f.parent.name, &cc, &pc); err != nil {
			return nil, err
		}
		f.cc, f.pc = cc, pc
		out[f.parent] = append(out[f.parent], f)
	}
	return out, rows.Err()
}

type erasePlan struct {
	tables map[tableKey]*tableInfo
	fks    map[tableKey][]fk
	title  string
}

// objectCondition returns the rows of an object table that will be deleted (and whose bodies go).
func (p *erasePlan) deleteCondition(t *tableInfo) string {
	cond := t.matchSQL(t.pidCols())
	if keepPublic[p.title] && t.key.String() == "datastore.objects" && t.has("permission") && t.has("upload_completed") && t.has("deleted") {
		cond = "(" + cond + " AND NOT (" + publicCourse + "))"
	}
	return cond
}

// planTables returns the tables to delete rows from, in a stable order.
func (p *erasePlan) deletable() []*tableInfo {
	var out []*tableInfo
	for _, t := range p.tables {
		if len(t.pidCols()) == 0 {
			continue
		}
		if cols, ok := anonymise[t.key.String()]; ok && len(cols) > 0 {
			continue
		}
		out = append(out, t)
	}
	sortTables(out)
	return out
}

func sortTables(ts []*tableInfo) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].key.String() < ts[j-1].key.String(); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// deleteWhere deletes the rows of t matching cond, children first. cond's only parameter is $1
// (the PIDs); extra is $2 when non-nil (bigint ids).
func (p *erasePlan) deleteWhere(ctx context.Context, x execer, t tableKey, cond string, args []any, depth int) (int, error) {
	n := 0
	if depth < 6 {
		for _, f := range p.fks[t] {
			if f.child == t {
				continue // a self reference is deleted with its own rows
			}
			var childCols, parentCols []string
			for i := range f.cc {
				childCols = append(childCols, pq.QuoteIdentifier(f.cc[i]))
				parentCols = append(parentCols, pq.QuoteIdentifier(f.pc[i]))
			}
			sub := fmt.Sprintf("(%s) IN (SELECT %s FROM %s WHERE %s)",
				strings.Join(childCols, ", "), strings.Join(parentCols, ", "), t.sql(), cond)
			c, err := p.deleteWhere(ctx, x, f.child, sub, args, depth+1)
			if err != nil {
				return 0, err
			}
			n += c
		}
	}
	res, err := x.ExecContext(ctx, "DELETE FROM "+t.sql()+" WHERE "+cond, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", t, err)
	}
	c, _ := res.RowsAffected()
	return n + int(c), nil
}

// scrubDatabase removes the PIDs' data from db. before is called with the data ids of the removed
// DataStore objects, before the transaction opens, so their bodies can leave the bucket first (a
// failed bucket call fails the erasure with the rows still in place, to be retried).
func scrubDatabase(ctx context.Context, db *sql.DB, title string, pids []string, before func(ids []int64) error) (scrubResult, error) {
	var res scrubResult
	tables, err := loadSchema(ctx, db)
	if err != nil {
		return res, err
	}
	fks, err := loadFKs(ctx, db)
	if err != nil {
		return res, err
	}
	p := &erasePlan{tables: tables, fks: fks, title: title}
	arg := pq.Array(pids)

	// 1. The ids of the objects that will be deleted (read only).
	idSet := map[int64]bool{}
	for _, t := range p.deletable() {
		if !t.pkData {
			continue
		}
		rows, err := db.QueryContext(ctx, "SELECT data_id FROM "+t.key.sql()+" WHERE "+p.deleteCondition(t), arg)
		if err != nil {
			return res, fmt.Errorf("%s: %w", t.key, err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return res, err
			}
			idSet[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
	}
	for id := range idSet {
		res.DataIDs = append(res.DataIDs, id)
	}
	if before != nil && len(res.DataIDs) > 0 {
		if err := before(res.DataIDs); err != nil {
			return res, err
		}
	}

	// 2. The rows, in one transaction.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Arrays first: the PID leaves other people's participant and recipient lists.
	for _, t := range p.sorted() {
		for _, c := range t.arrayCols() {
			q := pq.QuoteIdentifier(c.name)
			r, err := tx.ExecContext(ctx, fmt.Sprintf(
				"UPDATE %s SET %s = ARRAY(SELECT x FROM unnest(%s) AS x WHERE x::numeric <> ALL($1::numeric[])) WHERE %s::numeric[] && $1::numeric[]",
				t.key.sql(), q, q, q), arg)
			if err != nil {
				return res, fmt.Errorf("%s.%s: %w", t.key, c.name, err)
			}
			n, _ := r.RowsAffected()
			res.Rows += int(n)
		}
	}
	// Published courses stay, credited to nobody.
	for _, t := range p.sorted() {
		if keepPublic[title] && t.key.String() == "datastore.objects" && t.has("permission") && t.has("upload_completed") && t.has("deleted") {
			for _, c := range t.pidCols() {
				q := pq.QuoteIdentifier(c)
				r, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = 0 WHERE %s = ANY($1::numeric[]) AND %s",
					t.key.sql(), q, q, publicCourse), arg)
				if err != nil {
					return res, fmt.Errorf("%s: %w", t.key, err)
				}
				n, _ := r.RowsAffected()
				res.Rows += int(n)
			}
		}
	}
	// Tables that keep their rows with the PID set to 0.
	for _, t := range p.sorted() {
		for _, c := range anonymise[t.key.String()] {
			if !t.has(c) {
				continue
			}
			q := pq.QuoteIdentifier(c)
			r, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s = 0 WHERE %s = ANY($1::numeric[])", t.key.sql(), q, q), arg)
			if err != nil {
				return res, fmt.Errorf("%s.%s: %w", t.key, c, err)
			}
			n, _ := r.RowsAffected()
			res.Rows += int(n)
		}
	}
	// Rows of the player.
	for _, t := range p.deletable() {
		n, err := p.deleteWhere(ctx, tx, t.key, p.deleteCondition(t), []any{arg}, 0)
		if err != nil {
			return res, err
		}
		res.Rows += n
	}
	// Whatever else is keyed by the removed objects.
	if len(res.DataIDs) > 0 {
		ids := pq.Array(res.DataIDs)
		for _, t := range p.sorted() {
			if !t.hasData {
				continue
			}
			n, err := p.deleteWhere(ctx, tx, t.key, "data_id = ANY($1::bigint[])", []any{ids}, 0)
			if err != nil {
				return res, err
			}
			res.Rows += n
		}
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

func (p *erasePlan) sorted() []*tableInfo {
	out := make([]*tableInfo, 0, len(p.tables))
	for _, t := range p.tables {
		out = append(out, t)
	}
	sortTables(out)
	return out
}
