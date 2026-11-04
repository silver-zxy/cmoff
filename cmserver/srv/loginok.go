package srv

import (
	"cmserver/pepper"
	"cmserver/proto"
)

// buildLoginOk encodes a LoginOk (20375) body. The field *shape* is taken from
// the client's decoder at 0x78fdf4 (SERVER.md): 2 longs, 3 strings, 4 vints,
// string, 3 vints, 3 strings, vint, 6 strings, 3 vints, 2 string lists, bytes,
// 3 booleans, vint, string. Values that the client only stores are left empty
// or zero; the ones it checks (account id, home id) echo the login. Exact
// semantics of each scalar still need a capture to confirm, but the ordering
// and types match so the decoder consumes the whole body.
func buildLoginOk(l *pepper.Login, accountHi, accountLo int32) []byte {
	w := proto.NewWriter()

	// 2 longs: accountId, homeId. LogicLong is low-int first on the wire, to
	// match the client's own home encoder (writeInt(low), writeInt(high)).
	w.Int(accountLo)
	w.Int(accountHi)
	w.Int(accountLo)
	w.Int(accountHi)

	// 3 strings: passToken, facebookId, gamecenterId.
	w.String("cm-local-pass-token")
	w.StringNull()
	w.StringNull()

	// 4 vints: serverMajor, serverMinor, serverBuild, contentVersion.
	//
	// These MUST match the client's own version, or it treats the server as a
	// different build and goes into an update/patch flow that stalls the
	// loader (~50%). We parse the client's version out of its Login payload
	// (SERVER.md Login layout 0x31d45c: long accountId, string passToken, vint
	// major, minor, build, ...) and echo it back, so any build that connects
	// gets its own version returned.
	major, minor, build := int32(1), int32(0), int32(1143)
	lr := proto.NewReader(l.Payload)
	lr.Int()        // accountId high
	lr.Int()        // accountId low
	_ = lr.String() // passToken
	m0 := lr.VInt()
	m1 := lr.VInt()
	m2 := lr.VInt()
	if lr.Err() == nil {
		major, minor, build = m0, m1, m2
	}
	w.VInt(major)
	w.VInt(minor)
	w.VInt(build)
	w.VInt(0) // contentVersion

	// string: server environment. STRINGS.md lists Clash Mini's real envs as
	// "clashmini" / "clashmini-cn" / "clashmini-ed"; "prod" is a Clash Royale
	// leftover and not a valid Clash Mini environment, which may make the
	// client treat the account as non-production (-> deletion warning).
	w.String("clashmini")

	// 3 vints: session count, play-time seconds, days since created. (The
	// deletion warning was LoginOk +0x14c below, not these.)
	w.VInt(1)
	w.VInt(0)
	w.VInt(0)

	// 3 strings: facebookAppId, region, contentUrl.
	w.StringNull()
	w.String("US")
	w.StringNull()

	// vint: unknown.
	w.VInt(0)

	// 6 strings: country/region codes, chronos/shop urls, event asset url.
	w.StringNull()
	w.StringNull()
	w.StringNull()
	w.StringNull()
	w.StringNull()
	w.StringNull()

	// 3 vints: unknowns.
	w.VInt(0)
	w.VInt(0)
	w.VInt(0)

	// 2 string lists: empty.
	w.StringList(nil)
	w.StringList(nil)

	// bytes: empty blob.
	w.RawBytes(nil)

	// 3 booleans. (Tried all-true for the deletion-warning loop; no effect, so
	// back to the neutral false.)
	w.Bool(false)
	w.Bool(false)
	w.Bool(false)

	// vint at LoginOk object +0x14c: NOT server time. The LoginOk handler
	// (receive switch 0x4549a8, case 20375 -> 0x455b94) calls getter 0x5ac470
	// (`return obj->+0x14c`) at 0x457f50; any non-zero value opens dialog type
	// 19 via 0x8ff948 = TID_ACCOUNT_DELETION_WARNING (value = days until
	// deletion; <1 selects the "SOON" text). Sending time.Now() here was the
	// account-deletion loop. 0 = no deletion scheduled.
	w.VInt(0)

	// string: account creation date / misc.
	w.StringNull()

	return w.Bytes()
}
