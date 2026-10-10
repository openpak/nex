package nex

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
)

// Logs never carry a client address or a pid. Where two lines have to be
// matched up they carry logTag instead: a keyed hash under a per-process
// random key, so it correlates within one run and reverses to nothing.
// (This module cannot import github.com/openpak/nex/redact without a module
// cycle; the two produce the same shape of tag.)
var logTagKey = func() []byte {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic(err)
	}
	return k
}()

// logTag returns "~" and six hex digits standing in for v.
func logTag(v any) string {
	m := hmac.New(sha256.New, logTagKey)
	fmt.Fprint(m, v)
	return "~" + hex.EncodeToString(m.Sum(nil)[:3])
}

// logErr returns err's text without the addresses net errors spell out
// ("write udp [::]:60000->203.0.113.7:5000: ...").
func logErr(err error) string {
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Err != nil {
		return oe.Op + ": " + logErr(oe.Err)
	}
	var ae *net.AddrError
	if errors.As(err, &ae) {
		return "address error: " + ae.Err
	}
	return err.Error()
}
