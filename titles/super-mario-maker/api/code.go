// Package api is the read and bookmark API of the Super Mario Maker (Wii U) title, plus the
// Dothub-compatible pickup list the game's 100 Mario Challenge reads. See ../OPENPAK.md.
package api

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// accessKey is Super Mario Maker's NEX access key. The course code checksum is keyed with the
// MD5 of it (NintendoClients wiki, "Data Store Codes", Super Mario Maker, MIT).
const accessKey = "9f2b4678"

// maxDataID is the largest data ID a course code can carry: 48 bits (constants in
// database/init_postgres.go explain why the sequence stays below it).
const maxDataID = 1<<48 - 1

func checksum(dataID uint64) uint16 {
	key := md5.Sum([]byte(accessKey))
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], dataID)
	mac := hmac.New(md5.New, key[:])
	mac.Write(data[:])
	sum := mac.Sum(nil)
	// * The third and fourth byte of the HMAC, in reverse order
	return uint16(sum[3])<<8 | uint16(sum[2])
}

// CourseCode is the code the game shows for a course, "CCCC-0000-HHHH-LLLL": a checksum of the
// data ID, a zero field, then the 48-bit data ID.
func CourseCode(dataID uint64) string {
	return fmt.Sprintf("%04X-%04X-%04X-%04X", checksum(dataID), (dataID>>32)&0xFFFF, (dataID>>16)&0xFFFF, dataID&0xFFFF)
}

// ParseCourseCode accepts a course code (dashes optional, any case, checksum verified) or a
// plain decimal data ID, and returns the data ID.
func ParseCourseCode(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, "-") || len(s) == 16 {
		hex := strings.ToUpper(strings.ReplaceAll(s, "-", ""))
		if len(hex) != 16 {
			return 0, false
		}
		v, err := strconv.ParseUint(hex, 16, 64)
		if err != nil {
			return 0, false
		}
		id := v & 0xFFFFFFFFFFFF
		if id == 0 || uint16(v>>48) != checksum(id) {
			return 0, false
		}
		return id, true
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil || id == 0 || id > maxDataID {
		return 0, false
	}
	return id, true
}
