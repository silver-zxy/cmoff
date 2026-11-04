package srv

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"cmserver/home"
	"cmserver/pepper"
	"cmserver/proto"
)

// Options configure a Server.
type Options struct {
	Trace bool // log every frame (hex + ascii)
	// BattleOnline: the hook goes back online 1 s into a bot battle, so Exit
	// sends GoHome (default). false keeps the whole battle offline and makes
	// the hook reconnect afterwards (shows "connection lost").
	BattleOnline bool
	// SignalEmpty sends the battle signal as an empty KeepAliveServer (the
	// hook then always starts a duel) instead of padding it with the mode.
	SignalEmpty bool
}

// BattleSignalLen is the padding of the battle signal: the hook reads the
// values back from the frame length (cmoffline.c, feed()).
//
//	mode + 4*hold + 8*bot config + 128*bot hero level + 1408*bot mini level
//
// mode 2/3, hold 0/1, config 0-15, hero level 0-10, mini level 0-4 (0 =
// leave the bot's own levels).
func BattleSignalLen(mode int32, hold bool, cfg, hero, mini int32) int {
	n := int(mode%4) + 8*int(cfg%16) + 128*int(hero%11) + 1408*int(mini%5)
	if hold {
		n += 4
	}
	return n
}

// Server is the local game server for one account.
type Server struct {
	key  *pepper.Keypair
	acc  *home.Account
	opts Options

	mu      sync.Mutex
	pending battle // the bot battle the client is playing, until its result
}

// battle is a started bot battle: its mode and whether it counts (ranked).
type battle struct {
	mode   int32
	ranked bool
}

// HookBattleResult is not a game message: the client hook reports a bot
// battle's result on a connection of its own, as one plain frame with this
// id and the body (u8 mode, u8 place). See handleResult. Mode 9 instead says
// the tutorial is done, "place" being the chosen starting hero's
// support_heroes row (0 = unknown).
const HookBattleResult = 19999

const (
	hookTutorialDone = 9
	hookTutorialHome = 10 // the battle part is over; extra = tutorial flags
)

// tutorialDone: the new player picked a hero in the tutorial's first
// battle; the account gets that hero's starting chests (starting_choices.csv,
// again if the pick changes). The battle part is over: the home after it
// carries tutorial flags 0x200, its home part, which runs online until 607,
// 988 or the name end it.
func (s *Server) tutorialDone(hero int32) {
	s.acc.Lock()
	s.acc.TutorialFlags = 0x200
	var r *home.Reward
	var err error
	if s.acc.Progression {
		r, err = s.acc.StartWithHero(hero)
	}
	s.acc.Unlock()
	if err != nil {
		log.Printf("tutorial done: %v", err)
	}
	if err := s.acc.Save(); err != nil {
		log.Printf("save account: %v", err)
	}
	log.Printf("tutorial: starting hero %d", hero)
	if r != nil {
		log.Printf("   base collection: %s", r.String())
	}
}

// New makes a server with the given key and account.
func New(key *pepper.Keypair, acc *home.Account, opts Options) *Server {
	return &Server{key: key, acc: acc, opts: opts}
}

// ListenAndServe accepts clients on addr until the listener fails.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve accepts clients on ln.
func (s *Server) Serve(ln net.Listener) error {
	log.Printf("listening on %s", ln.Addr())
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	log.Printf("conn from %s", c.RemoteAddr())
	var sess *pepper.Session
	var shNonce []byte

	for {
		f, err := proto.ReadFrame(c)
		if err != nil {
			log.Printf("read: %v", err)
			return
		}
		// After LoginOk every client frame is secretbox'd on the stream. We
		// must open each one in arrival order so the client->server nonce
		// (decNonce, stepped +2 per message) stays aligned, even for frames
		// we don't act on yet.
		if sess.Ready() && f.ID != proto.ClientHello && f.ID != proto.Login {
			plain, derr := sess.Decrypt(f.Payload)
			if derr != nil {
				log.Printf("<- %d %s (%d bytes) DECRYPT FAILED: %v", f.ID, proto.Name(f.ID), len(f.Payload), derr)
				return
			}
			f.Payload = plain
			if s.opts.Trace {
				log.Printf("<- %d %s (%d bytes, decrypted)", f.ID, proto.Name(f.ID), len(plain))
				log.Printf("   hex   %s", hex.EncodeToString(plain))
				log.Printf("   ascii %s", printable(plain))
			}
		} else if s.opts.Trace {
			log.Printf("<- %d %s (%d bytes)", f.ID, proto.Name(f.ID), len(f.Payload))
			if sess == nil {
				log.Printf("   %s", hex.EncodeToString(f.Payload))
			}
		}
		if !s.opts.Trace && f.ID != proto.KeepAlive && f.ID != 12768 && f.ID != proto.EndClientTurn {
			// one line per message: shows what the client waits for
			log.Printf("<- %d %s (%d bytes)", f.ID, proto.Name(f.ID), len(f.Payload))
		}

		switch f.ID {
		case HookBattleResult:
			if sess == nil {
				s.handleResult(f.Payload)
				return
			}

		case proto.ClientHello:
			// reply so the client sends its Login. ServerHello body is the
			// server nonce written as a Supercell byte array: int32 length,
			// then the bytes.
			shNonce = make([]byte, 24)
			_, _ = rand.Read(shNonce)
			body := make([]byte, 4+len(shNonce))
			binary.BigEndian.PutUint32(body, uint32(len(shNonce)))
			copy(body[4:], shNonce)
			s.send(c, nil, proto.ServerHello, body)

		case proto.Login:
			l, err := pepper.OpenLogin(s.key, f.Payload, shNonce)
			if err != nil {
				log.Printf("login: %v (is the APK patched with this server's key?)", err)
				return
			}
			log.Printf("login ok: client pub %s", hex.EncodeToString(l.ClientPub[:]))
			if s.opts.Trace {
				log.Printf("   login payload (%d bytes): %s", len(l.Payload), hex.EncodeToString(l.Payload))
			}

			// LoginOk is itself crypto_box'd and establishes the stream key.
			replyBody, st, err := l.SealLoginReply(s.key, buildLoginOk(l, 0, 1))
			if err != nil {
				log.Printf("seal loginok: %v", err)
				return
			}
			sess = st
			if err := proto.WriteFrame(c, &proto.Frame{ID: proto.LoginOk, Payload: replyBody}); err != nil {
				log.Printf("write loginok: %v", err)
				return
			}
			// A new player in the tutorial: signal the hook (mode 1) before
			// the home, so the offline flag is on when the client starts the
			// tutorial's NPC battle (StartPvEMission 15409 is then handled
			// by the client itself; online it would wait for us forever).
			s.acc.Lock()
			// only the battle part runs offline; the home part (0x200,
			// hero choice) talks to us
			inTutorial := s.acc.TutorialPending && home.Tutorial && s.acc.TutorialFlags&0x200 == 0
			s.acc.Unlock()
			if inTutorial {
				log.Printf("-> tutorial signal")
				s.send(c, sess, proto.KeepAliveOk, make([]byte, 1))
			}
			// OwnHomeData keeps the client off the loading screen; it rides
			// the secretbox stream.
			s.sendHome(c, sess)

		case proto.KeepAlive:
			s.send(c, sess, proto.KeepAliveOk, nil)

		case proto.EndClientTurn:
			cmds, err := parseEndClientTurn(f.Payload)
			if err != nil {
				log.Printf("   commands: %v", err)
			}
			mode, ranked := apply(s.acc, cmds)
			s.pushChests(c, sess)
			if mode > 0 {
				// Battle signal for the client hook: a KeepAliveServer the
				// client did not ask for. Its body is padded with
				//   mode + 100 (keep the battle offline throughout)
				//        + 200 * bot config (bot_configs.csv, by trophies)
				// bytes; the hook reads it from the frame length
				// (KeepAliveServer's decoder reads nothing).
				s.acc.Lock()
				trophies := s.acc.Trophies
				hero, mini := s.acc.BotLevels()
				s.acc.Unlock()
				cfg := home.BotConfig(trophies)
				var pad []byte
				if !s.opts.SignalEmpty {
					pad = make([]byte, BattleSignalLen(mode, !s.opts.BattleOnline, cfg, hero, mini))
				}
				s.mu.Lock()
				s.pending = battle{mode: mode, ranked: ranked}
				s.mu.Unlock()
				kind := "casual"
				if ranked {
					kind = "ranked"
				}
				log.Printf("-> battle signal (mode %d, %s, bot config %d for %d trophies, %s, bot levels hero %d mini %d)",
					mode, kind, cfg, trophies, home.LeagueName(trophies), hero, mini)
				s.send(c, sess, proto.KeepAliveOk, pad)
			}

		case proto.AvatarNameCheckRequest:
			// Name dialog "OK": the client asks whether the name is valid.
			// AvatarNameCheckResponse (decoder 0x4515c4): bool invalid, int
			// error code, string name; invalid=false closes the dialog
			// (receive handler case 0x45612c -> 0x45719c). This build never
			// follows up with ChangeAvatarName or command 549, so the server
			// applies the name right here: save it and push
			// LogicChangeAvatarNameCommand 411, which sets the name and the
			// "name chosen" flag on the client.
			name := proto.NewReader(f.Payload).String()
			w := proto.NewWriter()
			w.Bool(false)
			w.Int(0)
			w.String(name)
			log.Printf("-> name check ok %q", name)
			s.send(c, sess, proto.AvatarNameCheckResp, w.Bytes())
			if name != "" {
				s.setName(c, sess, name)
			}

		case proto.ChangeAvatarName:
			// ChangeAvatarNameMessage (encoder 0x5a20b8): string name, bool.
			// Save it and confirm with the server command
			// LogicChangeAvatarNameCommand 411 in AvailableServerCommand.
			r := proto.NewReader(f.Payload)
			name := r.String()
			if r.Err() != nil || name == "" {
				log.Printf("   bad ChangeAvatarName")
				break
			}
			s.setName(c, sess, name)

		case proto.GoHome:
			// Exit after a bot battle: back to the (saved) home.
			s.sendHome(c, sess)

		default:
			// Everything else (device info 30000, latency 12768, ...) needs
			// no answer for offline play.
		}
	}
}

// handleResult applies the hook's report of the running bot battle. A
// duel's place is 1 (won) or 2 (lost); a Rumble's 1, 2, 3 (3rd-4th) or 5
// (5th-8th); 0 means the player left before the end. Only ranked battles
// change trophies; the next OwnHomeData (GoHome) shows them.
func (s *Server) handleResult(p []byte) {
	if len(p) < 2 {
		log.Printf("battle result: short report")
		return
	}
	mode, place := int32(p[0]), int32(p[1])
	var extra int32
	if len(p) >= 4 {
		extra = int32(p[2])<<8 | int32(p[3])
	}
	switch mode {
	case hookTutorialDone: // the hook saw the tutorial finish
		s.tutorialDone(place)
		return
	case hookTutorialHome:
		s.acc.Lock()
		s.acc.TutorialFlags = extra
		s.acc.Unlock()
		if err := s.acc.Save(); err != nil {
			log.Printf("save account: %v", err)
		}
		log.Printf("tutorial: battle part done (flags %#x), home part online", extra)
		return
	}
	s.mu.Lock()
	b := s.pending
	s.pending = battle{}
	s.mu.Unlock()
	if b.mode == 0 {
		log.Printf("battle result: %s, but no battle was started here", home.PlaceName(mode, place))
		return
	}
	if b.mode != mode {
		log.Printf("battle result: mode %d, but mode %d was started; ignored", mode, b.mode)
		return
	}
	s.acc.Lock()
	o := s.acc.BattleResult(mode, place, b.ranked)
	trophies, st := s.acc.Trophies, s.acc.Stats
	s.acc.Unlock()
	if err := s.acc.Save(); err != nil {
		log.Printf("save account: %v", err)
	}
	kind := "casual"
	if b.ranked {
		kind = fmt.Sprintf("ranked, %+d -> %d trophies, %s, streak %d",
			o.Trophies, trophies, home.LeagueName(trophies), st.RankedStreak)
		if o.StreakBonus != 0 {
			kind += fmt.Sprintf(", win streak bonus %+d included", o.StreakBonus)
		}
	}
	log.Printf("battle result: %s (%s)", home.PlaceName(mode, place), kind)
	if o.Reward.Gold > 0 || o.Reward.GoldCapped || len(o.Reward.Fragments) > 0 {
		log.Printf("   rewards: %s", o.Reward.String())
	}
}

// cmdServerChangeAvatarName is LogicChangeAvatarNameCommand, a server command.
const cmdServerChangeAvatarName = 411

// setName saves the name and pushes it to the client.
func (s *Server) setName(c net.Conn, sess *pepper.Session, name string) {
	s.acc.Lock()
	s.acc.SetName(name)
	s.acc.Unlock()
	if err := s.acc.Save(); err != nil {
		log.Printf("save account: %v", err)
	}
	log.Printf("-> name changed to %q", name)
	s.send(c, sess, proto.AvailableServerCommand, changeNameServerCommand(name))
}

// changeNameServerCommand builds AvailableServerCommandMessage (decoder
// 0x5929a0 -> 0x486c50: VInt type, then the command) carrying
// LogicChangeAvatarNameCommand 411 (decoder 0x746b48, execute 0x95088c):
// string name, int name changes (avatar +0x138), bool name set by the user
// (avatar +0x135), then the server command base (VInt, 0x54794c) and the
// LogicCommand base (0x90ab7c: VInt execute tick, -1 = now; VInt; account
// id as two VInts).
func changeNameServerCommand(name string) []byte {
	w := proto.NewWriter()
	w.VInt(cmdServerChangeAvatarName)
	w.String(name)
	w.Int(0) // keep renaming free
	w.Bool(true)
	w.VInt(0)
	w.VInt(-1)
	w.VInt(0)
	w.VInt(1)
	w.VInt(0)
	return w.Bytes()
}

// printable renders bytes as ASCII, non-printable shown as '.', for spotting
// command names and strings in a decrypted body.
func printable(p []byte) string {
	out := make([]byte, len(p))
	for i, b := range p {
		if b >= 0x20 && b < 0x7f {
			out[i] = b
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func (s *Server) send(c net.Conn, sess *pepper.Session, id uint16, body []byte) {
	if sess != nil {
		body = sess.Encrypt(body)
	}
	if err := proto.WriteFrame(c, &proto.Frame{ID: id, Payload: body}); err != nil {
		log.Printf("write %d: %v", id, err)
	}
}

// sendHome sends the saved home and logs the chests in it.
func (s *Server) sendHome(c net.Conn, sess *pepper.Session) {
	s.acc.Lock()
	var list []string
	for _, ch := range s.acc.Chests {
		how := "silent"
		if ch.Show {
			how = "popup"
		}
		list = append(list, fmt.Sprintf("%d #%d %s (%s)", ch.Chest, ch.Index, how, ch.Content))
	}
	s.acc.ClearPushes()
	s.acc.Unlock()
	if len(list) > 0 {
		log.Printf("-> home with %d chests to open: %s", len(list), strings.Join(list, "; "))
	}
	if q := s.acc.QuestSummary(); q != "" {
		log.Printf("-> home with quests: %s", q)
	}
	s.send(c, sess, proto.OwnHomeData, s.acc.OwnHomeData())
}

// cmdServerClaimReward is LogicClaimRewardCommand 420, a server command
// (decoder 0x4dad24, execute 0x6a5c38): the content of a chest the client
// made itself, by its index.
const cmdServerClaimReward = 420

// pushChests sends the content of the chests the client has just made
// itself (home.Account.TakePushes).
func (s *Server) pushChests(c net.Conn, sess *pepper.Session) {
	s.acc.Lock()
	pushes := s.acc.TakePushes()
	s.acc.Unlock()
	for _, ch := range pushes {
		log.Printf("-> content of chest %d #%d (%s)", ch.Chest, ch.Index, ch.Content)
		s.send(c, sess, proto.AvailableServerCommand, claimRewardServerCommand(ch))
	}
}

// claimRewardServerCommand: 420 in AvailableServerCommand: the content
// (0x37fee8), VInt chest index (+0x28), the server command base and the
// LogicCommand base as for 411.
func claimRewardServerCommand(ch home.PendingChest) []byte {
	w := proto.NewWriter()
	w.VInt(cmdServerClaimReward)
	ch.EncodeContent(w)
	w.VInt(ch.Index)
	w.VInt(0)
	w.VInt(-1)
	w.VInt(0)
	w.VInt(1)
	w.VInt(0)
	return w.Bytes()
}

// cmdServerUpdateChronosEvents is LogicUpdateChronosEventsCommand 417, a
// server command (decoder 0x461524: the server command base first, then a
// bool and the event collection 0x295f9c; execute 0x682b80).
const cmdServerUpdateChronosEvents = 417

// sendEvents sends the live events (home.LiveEvents) with 417.
func (s *Server) sendEvents(c net.Conn, sess *pepper.Session) {
	events := home.LiveEvents()
	if len(events) == 0 {
		return
	}
	for _, e := range events {
		log.Printf("-> live event %d %q (type %d): %s", e.ID, e.Name, e.Type, e.Parameters)
	}
	s.send(c, sess, proto.AvailableServerCommand, chronosEventsServerCommand(events))
}

func chronosEventsServerCommand(events []home.LiveEvent) []byte {
	w := proto.NewWriter()
	w.VInt(cmdServerUpdateChronosEvents)
	w.VInt(0) // server command base
	w.VInt(-1)
	w.VInt(0)
	w.VInt(1)
	w.VInt(0) // LogicCommand base
	w.Bool(true)
	home.EncodeEvents(w, events)
	return w.Bytes()
}
