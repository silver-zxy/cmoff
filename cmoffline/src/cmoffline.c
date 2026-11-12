/* cmoffline - client hook for Clash Mini 1.1143.2 (arm64-v8a).
 *
 * Loaded into the game through a DT_NEEDED entry on libsentry-android.so
 * (tools/patch_apk.py). It:
 *   - points game.prod.clashminigame.com at 127.0.0.1 (libg's GOT slot of
 *     getaddrinfo), where cmserver runs;
 *   - starts the client's own offline bot battle when the server signals it,
 *     and keeps renaming enabled (see battles_main, docs/CLIENT_HOOK.md);
 *   - with -DCMOFFLINE_LOCAL_SERVER -DSERVER_KEY_HEX='"..."': starts
 *     cmserver itself (libcmserver.so) inside the game, so no PC is needed;
 *   - logs crashes with Ghidra addresses (unless -DCMOFFLINE_NO_CRASH_LOGGER).
 * -DCMOFFLINE_FORCE_OFFLINE is the old client-only mode (offline flag always
 * set, empty deck, no server). Ghidra address = libg offset + 0x100000. */

#define _GNU_SOURCE
#include <android/log.h>
#include <dlfcn.h>
#include <elf.h>
#include <link.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <pthread.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/auxv.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/uio.h>
#include <time.h>
#include <ucontext.h>
#include <unistd.h>

#define TAG "cmoffline"
#define LOG(...) __android_log_print(ANDROID_LOG_INFO, TAG, __VA_ARGS__)

#define GAME_HOST          "game.prod.clashminigame.com"
#ifndef REDIRECT_TO
#define REDIRECT_TO        "127.0.0.1"
#endif
#define LIBG_BUILD_ID      "128b8fb1640843833d20eb35438f69decd98f8df"

#define LIBG_GOT_GETADDRINFO  0xdf4fc8
#define LIBG_OFFLINE_FLAG     0x108abd8
#define LIBG_SIZE             0x2000000
#define GHIDRA_BASE           0x100000

static uintptr_t libg_base;
static char libg_build_id[41];

typedef int (*getaddrinfo_fn)(const char *, const char *,
                              const struct addrinfo *, struct addrinfo **);
static getaddrinfo_fn real_getaddrinfo;

#ifndef CMOFFLINE_NO_CRASH_LOGGER
static struct sigaction old_segv, old_bus, old_abrt;
#endif

static void read_build_id(const struct dl_phdr_info *info) {
    libg_build_id[0] = 0;
    for (int i = 0; i < info->dlpi_phnum; i++) {
        const ElfW(Phdr) *ph = &info->dlpi_phdr[i];
        if (ph->p_type != PT_NOTE) continue;
        const uint8_t *p = (const uint8_t *)(info->dlpi_addr + ph->p_vaddr);
        const uint8_t *end = p + ph->p_memsz;
        while (p + sizeof(ElfW(Nhdr)) <= end) {
            const ElfW(Nhdr) *n = (const ElfW(Nhdr) *)p;
            const uint8_t *name = p + sizeof *n;
            const uint8_t *desc = name + ((n->n_namesz + 3) & ~3u);
            if (n->n_type == NT_GNU_BUILD_ID && n->n_namesz == 4 &&
                memcmp(name, "GNU", 4) == 0 && n->n_descsz <= 20) {
                for (unsigned j = 0; j < n->n_descsz; j++)
                    snprintf(libg_build_id + 2 * j, 3, "%02x", desc[j]);
                return;
            }
            p = desc + ((n->n_descsz + 3) & ~3u);
        }
    }
}

static int find_libg(struct dl_phdr_info *info, size_t size, void *data) {
    (void)size; (void)data;
    if (!info->dlpi_name || !strstr(info->dlpi_name, "/libg.so")) return 0;
    libg_base = info->dlpi_addr;
    read_build_id(info);
    return 1;
}

#define G(off) ((void *)(libg_base + (off)))

static int cm_getaddrinfo(const char *node, const char *service,
                          const struct addrinfo *hints, struct addrinfo **res) {
    if (node && strcmp(node, GAME_HOST) == 0) {
        LOG("redirecting %s:%s to %s", node, service ? service : "", REDIRECT_TO);
        node = REDIRECT_TO;
    }
    return real_getaddrinfo(node, service, hints, res);
}

static int patch_pointer(void **slot, void *value) {
    unsigned long page = getauxval(AT_PAGESZ);
    void *start = (void *)((uintptr_t)slot & ~((uintptr_t)page - 1));
    if (mprotect(start, page, PROT_READ | PROT_WRITE)) return -1;
    *slot = value;
    mprotect(start, page, PROT_READ);
    return 0;
}



#ifndef CMOFFLINE_NO_CRASH_LOGGER
static void log_addr(const char *what, uintptr_t a) {
    if (libg_base && a >= libg_base && a < libg_base + LIBG_SIZE)
        LOG("  %s libg+%#lx (Ghidra %#lx)", what,
            (unsigned long)(a - libg_base), (unsigned long)(a - libg_base + GHIDRA_BASE));
    else
        LOG("  %s %#lx", what, (unsigned long)a);
}

static void on_crash(int sig, siginfo_t *info, void *ctx) {
    ucontext_t *uc = ctx;
    LOG("signal %d, fault addr %p", sig, info ? info->si_addr : NULL);
    log_addr("pc", uc->uc_mcontext.pc);
    log_addr("lr", uc->uc_mcontext.regs[30]);

    uintptr_t *fp = (uintptr_t *)uc->uc_mcontext.regs[29];
    for (int i = 0; i < 16 && fp && !((uintptr_t)fp & 7); i++) {
        uintptr_t *next = (uintptr_t *)fp[0];
        log_addr("frame", fp[1]);
        if (next <= fp) break;
        fp = next;
    }

    struct sigaction *old = sig == SIGSEGV ? &old_segv : sig == SIGBUS ? &old_bus : &old_abrt;
    sigaction(sig, old, NULL);
    if (old->sa_flags & SA_SIGINFO) {
        if (old->sa_sigaction) old->sa_sigaction(sig, info, ctx);
    } else if (old->sa_handler != SIG_DFL && old->sa_handler != SIG_IGN) {
        old->sa_handler(sig);
    }
}

static void install_crash_logger(void) {
    struct sigaction sa;
    memset(&sa, 0, sizeof sa);
    sa.sa_sigaction = on_crash;
    sa.sa_flags = SA_SIGINFO | SA_ONSTACK;
    sigaction(SIGSEGV, &sa, &old_segv);
    sigaction(SIGBUS,  &sa, &old_bus);
    sigaction(SIGABRT, &sa, &old_abrt);
}
#else
static void install_crash_logger(void) {}
#endif

/* ------------------------------------------------------------------------
 * Bot battles (online mode). The home stays online so deck/hero/name commands
 * reach the server. When the player presses Duel the client sends
 * LogicStartMatchmakeCommand 582; the server answers with a KeepAliveServer
 * (20108) the client never asked for. We watch the plaintext frame headers
 * (id u16, length u24, version u16) on the game socket, and on that signal
 * switch the client's own offline duel on (PORTING.md §6):
 *   offline flag = 1, gamemode+0x15c = mode, gamemode+0x160 = 0
 * Home update 0x467e78 then builds the battle vs OfflineMatchNPC (0x940664).
 * One second into the battle (state 5) the flag goes off again, so Exit sends
 * GoHome and the server answers with OwnHomeData.
 *
 * The server pads the signal's body with mode + 100 * hold + 200 * bot
 * config bytes, so the hook reads them from the ciphertext length minus the
 * 16-byte MAC. An empty unrequested 20108 (more replies than KeepAlives
 * sent) also counts, as mode 2 (duel) with a random bot.
 * ------------------------------------------------------------------------ */
#ifndef CMOFFLINE_NO_BATTLES

#define LIBG_GOT_READ      0xdf4c08
#define LIBG_GOT_RECVFROM  0xdf43e8
#define LIBG_GOT_WRITE     0xdf6d18
#define LIBG_GOT_SENDTO    0xdf4bf0
#define LIBG_GOT_CONNECT   0xdf55a8
#define LIBG_GOT_CLOSE     0xdf49a8
#define LIBG_GAME_MAIN     0x1078728   /* Ghidra 0x1178728: GameMain* */
#define GM_STATE           0x28        /* 0x4e68f4: 4 = home, 5 = battle */
#define GM_MODE            0x20        /* 0x5d6a94: game mode object */
#define MODE_MATCH         0x15c       /* listener 0x59e8a0 */
#define MODE_MATCH_FLAG    0x160
#define LIBG_GLOBALS       0x1080760   /* Ghidra 0x1180760: LogicGlobals* (getter 0x933020) */
#define GLB_NAME_CHANGE    0x111       /* ENABLE_NAME_CHANGE (loader 0x4b0790); not in globals.csv -> false */

/* The offline bot is npcs.csv row OfflineMatchNPC with BotConfigIndex 0:
 * tutorial decks, dynamic difficulty, holds elixir back. Before each bot
 * battle we rewrite that cell of the loaded CSV table with a real config
 * (the battle builder 0x940664 reads it at 0x940c40). Its Seed column is
 * never read (getSeed 0x8344dc has no callers); the battle already seeds
 * srand() from clock_gettime (0x940734 -> 0x4d273c). Layout:
 *   npc table  = *(libg+0x10807d8)  (getter 0x92198c: vfunc +0x40 by name;
 *                vfunc +0x30 count, +0x38 item(i), as in 0x4db698)
 *   npc data +0x10 = CSVRow*: row->table (+0), row->index (+8)
 *   table+0x10 = columns[], column+0x18 = int cells   (0x535e08 -> 0x72c69c
 *                -> 0x817a60; 0x7fffffff = empty)
 *   column index of Seed / BotConfigIndex: globals Ghidra 0x10bc300 /
 *                0x1057740 (set by the npc loader 0x8c28f0 / 0x8c2b44;
 *                getters getSeed 0x8344dc, getBotConfigIndex 0x508d5c) */
#define LIBG_NPC_TABLE     0x10807d8
#define LIBG_COL_BOTCONFIG 0xf57740
#define NPC_OFFLINE_ROW    3           /* FirstMatch, SecondMatch, ThirdMatch, OfflineMatch */
/* Bot decks (bot_decks.csv, data class 133, table *(libg+0x1080b70)) carry
 * their own levels, by league: heroes 1-10, minis 1-4 (Bronze low,
 * Grandmaster and the FF decks maxed), as in the original. The deck object
 * caches them at load (ctor 0x5b49ac): hero level at +0x40, mini levels as
 * an int list at +0x58 with its count at +0x64 (minis list +0x48/+0x54).
 * The hook keeps those levels; only when the server sends levels (the
 * unlocked collection, no progression) it sets every deck to them. The
 * generated decks (BOT_DECK_NORMAL / NEWBIE / RANDOM) are not in the table. */
#define LIBG_BOTDECK_TABLE 0x1080b70
/* Boards (locations.csv, table *(libg+0x10807c0), looked up by 0x82312c):
 * items WindyPlains 0, VampireCastle 1 (Banquet Hall), ElectroValley 2,
 * HealingTemple 3, FishTown 4 (each two CSV lines: their row indexes are
 * 0, 2, 4, 6, 8). A PvE battle plays on its npc's board,
 * resolved at load into npc +0x38 (0x47d8dc, "No location in NPC level");
 * the game mode's own boards are not used for it. The server picks one of
 * the league's Season 1 boards and sends item + 1 (0: the npc's own). */
#define LIBG_LOCATION_TABLE 0x10807c0
#define NPC_LOCATION        0x38
#define BOTDECK_MAX        256
#define BOTDECK_MINIS      8
#ifndef BOT_CONFIGS
/* Used only when the server sends no config (it picks one by trophies).
 * bot_configs.csv: 5 Silver3, 6 Silver2, 7 Silver1, 8 Gold3, 9 Gold2,
 * 10 Diamond3; all BotNormal with real deck pools. For 0-10 the config's
 * name equals its position, so these are right whether the game looks the
 * index up by position or by name (13 = Master only by name). */
#define BOT_CONFIGS 5, 6, 7, 8, 9, 10
#endif
#define GAME_PORT          9339
#define MSG_KEEPALIVE      10108
#define MSG_KEEPALIVE_OK   20108
#define MAC_LEN            16

typedef ssize_t (*read_fn)(int, void *, size_t);
typedef ssize_t (*write_fn)(int, const void *, size_t);
typedef ssize_t (*recvfrom_fn)(int, void *, size_t, int, struct sockaddr *, socklen_t *);
typedef ssize_t (*sendto_fn)(int, const void *, size_t, int, const struct sockaddr *, socklen_t);
typedef int (*connect_fn)(int, const struct sockaddr *, socklen_t);
typedef int (*close_fn)(int);

static read_fn real_read;
static write_fn real_write;
static recvfrom_fn real_recvfrom;
static sendto_fn real_sendto;
static connect_fn real_connect;
static close_fn real_close;

static volatile int game_fd = -1;
static volatile int keepalives_out;     /* KeepAlives sent, not yet answered */
static volatile int pending_mode;       /* >0: start an offline battle */
static volatile int pending_hold;       /* keep the offline flag for the whole battle */
static volatile int pending_bot;        /* bot config from the server, 0 = random */
static volatile int pending_hero_level; /* bot levels from the server, 0 = the decks' own */
static volatile int pending_board;      /* locations item + 1 from the server, 0 = the npc's own */
static volatile int pending_mini_level;
static volatile int battle_mode;        /* mode of the battle being played */
static volatile int tutorial;           /* the client plays its tutorial offline */

struct framer { uint8_t hdr[7]; int have; uint32_t skip; };
static struct framer in_fr, out_fr;
static pthread_mutex_t fr_lock = PTHREAD_MUTEX_INITIALIZER;

static void feed(struct framer *fr, const uint8_t *p, size_t n, int incoming) {
    while (n) {
        if (fr->skip) {
            size_t k = n < fr->skip ? n : fr->skip;
            fr->skip -= k; p += k; n -= k;
            continue;
        }
        fr->hdr[fr->have++] = *p++; n--;
        if (fr->have < 7) continue;
        fr->have = 0;
        unsigned id = (unsigned)fr->hdr[0] << 8 | fr->hdr[1];
        uint32_t len = (uint32_t)fr->hdr[2] << 16 | (uint32_t)fr->hdr[3] << 8 | fr->hdr[4];
        fr->skip = len;
        if (!incoming) {
            if (id == MSG_KEEPALIVE) __atomic_add_fetch(&keepalives_out, 1, __ATOMIC_SEQ_CST);
            continue;
        }
        if (id != MSG_KEEPALIVE_OK) continue;
        if (len > MAC_LEN) {
            /* padding = mode + 4*hold + 8*bot config + 128*bot hero
             * level + 1408*bot mini level + 7040*board (cmserver
             * BattleSignalLen) */
            int v = (int)(len - MAC_LEN);
            if (v == 1) {
                /* mode 1: a new player's tutorial. Its NPC battles are built
                 * by the client itself only while the offline flag is set
                 * (StartPvEMission 15409 offline: 0x87641c -> 0x940664);
                 * online it waits for the server's battle forever (50 %). */
                tutorial = 1;
                *(volatile uint8_t *)G(LIBG_OFFLINE_FLAG) = 1;
                LOG("tutorial signal from server: offline until the tutorial is done");
                continue;
            }
            pending_mode = v % 4 ? v % 4 : 2;
            pending_hold = v / 4 % 2;
            pending_bot = v / 8 % 16;
            pending_hero_level = v / 128 % 11;
            pending_mini_level = v / 1408 % 5;
            pending_board = v / 7040 % 8;
            LOG("battle signal from server, mode %d, bot config %d, bot levels hero %d mini %d, board %d%s",
                pending_mode, pending_bot, pending_hero_level, pending_mini_level, pending_board,
                pending_hold ? ", hold" : "");
        } else if (__atomic_sub_fetch(&keepalives_out, 1, __ATOMIC_SEQ_CST) < 0) {
            keepalives_out = 0;
            pending_hold = 0;
            pending_bot = 0;
            pending_hero_level = pending_mini_level = pending_board = 0;
            pending_mode = 2;
            LOG("unrequested KeepAliveServer -> duel");
        }
    }
}

static void reset_framers(void) {
    pthread_mutex_lock(&fr_lock);
    memset(&in_fr, 0, sizeof in_fr);
    memset(&out_fr, 0, sizeof out_fr);
    keepalives_out = 0;
    pthread_mutex_unlock(&fr_lock);
}

static void on_data(int fd, const void *buf, ssize_t n, int incoming) {
    if (n <= 0 || fd != game_fd) return;
    pthread_mutex_lock(&fr_lock);
    feed(incoming ? &in_fr : &out_fr, buf, (size_t)n, incoming);
    pthread_mutex_unlock(&fr_lock);
}

static ssize_t cm_read(int fd, void *buf, size_t n) {
    ssize_t r = real_read(fd, buf, n);
    on_data(fd, buf, r, 1);
    return r;
}
static ssize_t cm_recvfrom(int fd, void *buf, size_t n, int fl, struct sockaddr *a, socklen_t *al) {
    ssize_t r = real_recvfrom(fd, buf, n, fl, a, al);
    if (!(fl & MSG_PEEK)) on_data(fd, buf, r, 1);
    return r;
}
static ssize_t cm_write(int fd, const void *buf, size_t n) {
    ssize_t r = real_write(fd, buf, n);
    on_data(fd, buf, r, 0);
    return r;
}
static ssize_t cm_sendto(int fd, const void *buf, size_t n, int fl, const struct sockaddr *a, socklen_t al) {
    ssize_t r = real_sendto(fd, buf, n, fl, a, al);
    on_data(fd, buf, r, 0);
    return r;
}
static int cm_connect(int fd, const struct sockaddr *a, socklen_t al) {
    int r = real_connect(fd, a, al);
    unsigned port = 0;
    if (a && a->sa_family == AF_INET)  port = ntohs(((const struct sockaddr_in *)a)->sin_port);
    if (a && a->sa_family == AF_INET6) port = ntohs(((const struct sockaddr_in6 *)a)->sin6_port);
    if (port == GAME_PORT) {
        reset_framers();
        game_fd = fd;
        LOG("game socket fd %d", fd);
    }
    return r;
}
static int cm_close(int fd) {
    if (fd == game_fd) { game_fd = -1; LOG("game socket closed"); }
    return real_close(fd);
}

static int hook_got(uintptr_t off, const char *sym, void *fn, void **real) {
    void **slot = G(off);
    void *want = dlsym(RTLD_DEFAULT, sym);
    for (int i = 0; i < 2000 && *slot != want; i++) usleep(1000);
    if (!*slot) { LOG("GOT %s empty", sym); return -1; }
    *real = *slot;
    if (patch_pointer(slot, fn)) { LOG("GOT %s: mprotect failed", sym); return -1; }
    return 0;
}

static int *npc_cell(uint8_t *row, uint32_t col_global) {
    uint8_t *table = *(uint8_t **)row;
    int index = *(int32_t *)(row + 8);
    int col = *(int32_t *)G(col_global);
    if (!table || index < 0 || col < 0 || col > 64) return NULL;
    uint8_t **cols = *(uint8_t ***)(table + 0x10);
    if (!cols || !cols[col]) return NULL;
    int32_t *cells = *(int32_t **)(cols[col] + 0x18);
    return cells ? &cells[index] : NULL;
}

/* The decks' own levels, saved before the hook first changes them. */
static int botdeck_saved;
static int botdeck_count;
static int32_t botdeck_hero[BOTDECK_MAX];
static int32_t botdeck_minis[BOTDECK_MAX][BOTDECK_MINIS];

/* Bot deck levels for the next battle: the server's (hero, mini 1-10 / 1-4)
 * for every deck, or with 0 the decks' own levels from bot_decks.csv. */
static void level_bot_decks(int hero_level, int mini_level) {
    int own = hero_level < 1 || hero_level > 10 || mini_level < 1 || mini_level > 4;
    uint8_t *table = *(uint8_t **)G(LIBG_BOTDECK_TABLE);
    if (!table) { LOG("bot: deck table not loaded"); return; }
    void **vt = *(void ***)table;
    int count = ((int (*)(void *))vt[0x30 / 8])(table);
    if (count > BOTDECK_MAX) count = BOTDECK_MAX;
    if (!botdeck_saved) {
        if (own) return; /* nothing changed yet: the decks have their own */
        for (int i = 0; i < count; i++) {
            uint8_t *deck = ((uint8_t *(*)(void *, int))vt[0x38 / 8])(table, i);
            if (!deck) continue;
            botdeck_hero[i] = *(int32_t *)(deck + 0x40);
            int32_t *levels = *(int32_t **)(deck + 0x58);
            int n = *(int32_t *)(deck + 0x64);
            for (int j = 0; levels && j < n && j < BOTDECK_MINIS; j++)
                botdeck_minis[i][j] = levels[j];
        }
        botdeck_count = count;
        botdeck_saved = 1;
    }
    int decks = 0;
    for (int i = 0; i < count && i < botdeck_count; i++) {
        uint8_t *deck = ((uint8_t *(*)(void *, int))vt[0x38 / 8])(table, i);
        if (!deck) continue;
        int32_t *hero = (int32_t *)(deck + 0x40);
        if (botdeck_hero[i] > 0 && botdeck_hero[i] <= 10)
            *hero = own ? botdeck_hero[i] : hero_level;
        int32_t *levels = *(int32_t **)(deck + 0x58);
        int n = *(int32_t *)(deck + 0x64);
        for (int j = 0; levels && j < n && j < BOTDECK_MINIS; j++)
            if (botdeck_minis[i][j] > 0 && botdeck_minis[i][j] <= 4)
                levels[j] = own ? botdeck_minis[i][j] : mini_level;
        decks++;
    }
    if (own) LOG("bot: %d decks back at their own levels", decks);
    else LOG("bot: %d decks set to hero level %d, mini level %d", decks, hero_level, mini_level);
}

/* The bot strength for the next offline battle: the server's choice (by our
 * trophies), else a random one from BOT_CONFIGS. */
static void choose_bot(int config) {
    static const int configs[] = { BOT_CONFIGS };
    uint8_t *table = *(uint8_t **)G(LIBG_NPC_TABLE);
    if (!table) { LOG("bot: npc table not loaded"); return; }
    void **vt = *(void ***)table;
    int count = ((int (*)(void *))vt[0x30 / 8])(table);
    if (count <= NPC_OFFLINE_ROW) { LOG("bot: npc table has %d rows", count); return; }
    uint8_t *npc = ((uint8_t *(*)(void *, int))vt[0x38 / 8])(table, NPC_OFFLINE_ROW);
    uint8_t *row = npc ? *(uint8_t **)(npc + 0x10) : NULL;
    if (!row || *(int32_t *)(row + 8) != NPC_OFFLINE_ROW) { LOG("bot: OfflineMatchNPC not found"); return; }
    int *cfg = npc_cell(row, LIBG_COL_BOTCONFIG);
    if (!cfg) { LOG("bot: config cell not found"); return; }
    static unsigned rng;
    if (!rng) rng = (unsigned)time(NULL) ^ (unsigned)getpid();
    rng = rng * 1103515245u + 12345u;
    *cfg = config > 0 ? config : configs[(rng >> 16) % (sizeof configs / sizeof configs[0])];
    LOG("bot: config %d (bot_configs.csv)%s", *cfg, config > 0 ? "" : ", random");
}

/* The board of the next bot battle: locations item board-1, or with 0
 * the npc's own (saved the first time it is changed). */
static void choose_board(int board) {
    static uint8_t *own;
    static int saved;
    uint8_t *npcs = *(uint8_t **)G(LIBG_NPC_TABLE);
    uint8_t *locs = *(uint8_t **)G(LIBG_LOCATION_TABLE);
    if (!npcs || !locs) { LOG("board: tables not loaded"); return; }
    void **nvt = *(void ***)npcs, **lvt = *(void ***)locs;
    if (((int (*)(void *))nvt[0x30 / 8])(npcs) <= NPC_OFFLINE_ROW) return;
    uint8_t *npc = ((uint8_t *(*)(void *, int))nvt[0x38 / 8])(npcs, NPC_OFFLINE_ROW);
    if (!npc) return;
    uint8_t **slot = (uint8_t **)(npc + NPC_LOCATION);
    if (!saved) {
        if (board <= 0) return; /* nothing changed yet */
        own = *slot;
        saved = 1;
    }
    uint8_t *loc = own;
    if (board > 0) {
        /* item board-1 of the table: the boards in the CSV's order. Each
         * board spans two CSV lines, so the row index (item +0x10, +8) is
         * the line, 2 * item: it is only logged. */
        int count = ((int (*)(void *))lvt[0x30 / 8])(locs);
        if (board > count) { LOG("board: item %d of %d", board - 1, count); return; }
        loc = ((uint8_t *(*)(void *, int))lvt[0x38 / 8])(locs, board - 1);
        if (!loc) return;
        uint8_t *row = *(uint8_t **)(loc + 0x10);
        LOG("board: item %d, CSV line %d", board - 1, row ? *(int32_t *)(row + 8) : -1);
    }
    *slot = loc;
    LOG("board: %s", board > 0 ? "the server's" : "the npc's own");
}

/* ---- battle result -------------------------------------------------------
 * The server never learns who won: GoHome is the same for a win and a loss.
 * The hook reads it from the logic battle the moment the end screen exists
 * (the battle screen's +0x1a0, created by 0x8ebf68; later the battle is
 * torn down and reads as zeros), and reports it to the server.
 *   screen = *(libg+0x1085428)          (0x5b7f84)
 *   battle = *(*(*(screen + 0x100) + 0x68) + 0x98)    (0x8dcb90)
 *   battle+0x18 ptr[] players, count battle+0x58; player id hi/lo at +0/+4
 *                     (bots have negative ids, we are the one that is not)
 *   battle+0xb0 ptr[] player states, same order
 * Duel, as the end screen 0x945098 decides it: won when the rounds won
 * (state+0x178) reach the rounds needed (*(battle+0x108)+0xe0).
 * Rumble (found by comparing all players' states over four Rumbles):
 *   state+0xb4 the last stage reached (3 = last 8, 4 = last 4, 5 = final)
 *   state+0xc0 the stage the player was knocked out in, -1 = never
 * The battle's own winner index (+0x98, getWinnerAvatar) stays 0 offline. */
#define LIBG_BATTLE_ROOT   0x1085428
#define SCREEN_LOGIC_OWNER 0x100
#define SCREEN_END_SCREEN  0x1a0
#define LOGIC_BATTLE_OWNER 0x68
#define LOGIC_BATTLE       0x98
#define BATTLE_PLAYERS     0x18
#define BATTLE_PLAYER_N    0x58
#define BATTLE_STATES      0xb0
#define BATTLE_RULES       0x108
#define RULES_WINS_NEEDED  0xe0
#define STATE_WINS         0x178
#define STATE_OUT_STAGE    0xc0
#define HOOK_BATTLE_RESULT 19999       /* cmserver srv.HookBattleResult */

/* Copy memory that may be unmapped without crashing. Raw syscall: bionic
 * only declares process_vm_readv from API 23, we build for 21. */
static int peek(const void *addr, void *out, size_t n) {
    struct iovec local = { out, n }, remote = { (void *)addr, n };
    return addr && syscall(__NR_process_vm_readv, getpid(), &local, 1UL, &remote, 1UL, 0UL)
                       == (long)n ? 0 : -1;
}
static uint8_t *peek_ptr(const uint8_t *base, size_t off) {
    uint8_t *p = NULL;
    return base && !peek(base + off, &p, sizeof p) ? p : NULL;
}
static int32_t peek_int(const uint8_t *base, size_t off, int32_t fallback) {
    int32_t v;
    return base && !peek(base + off, &v, sizeof v) ? v : fallback;
}

/* Rounds of the last battle read, for the quests ("win N rounds", 3:0):
 * ours and, in a duel, the opponent's (each player's state +0x178). The
 * client counts such quests only in its own home, not in the offline
 * battle's, so the server needs them. -1 = unknown. */
static int rounds_won = -1, rounds_lost = -1;

/* Our place: duel 1 won / 2 lost; Rumble 1, 2, 3 (3rd-4th), 5 (5th-8th).
 * 0 if the battle cannot be read. */
static int read_place(int mode, uint8_t *screen) {
    uint8_t *b = peek_ptr(peek_ptr(peek_ptr(screen, SCREEN_LOGIC_OWNER), LOGIC_BATTLE_OWNER), LOGIC_BATTLE);
    int n = peek_int(b, BATTLE_PLAYER_N, 0);
    if (!b || n < 1 || n > 8) return 0;
    int me = -1;
    for (int i = 0; i < n && me < 0; i++)
        if (peek_int(peek_ptr(b, BATTLE_PLAYERS + i * 8), 0, -1) >= 0) me = i;
    uint8_t *st = me < 0 ? NULL : peek_ptr(b, BATTLE_STATES + me * 8);
    if (!st) return 0;
    rounds_won = peek_int(st, STATE_WINS, -1);
    rounds_lost = -1;
    if (mode != 3 && n == 2)
        rounds_lost = peek_int(peek_ptr(b, BATTLE_STATES + (1 - me) * 8), STATE_WINS, -1);
    if (mode != 3) {
        int wins = peek_int(st, STATE_WINS, -1);
        int needed = peek_int(peek_ptr(b, BATTLE_RULES), RULES_WINS_NEEDED, -1);
        if (wins < 0 || needed <= 0) return 0;
        LOG("battle result: duel %s (rounds won %d of %d, lost %d)", wins >= needed ? "won" : "lost",
            wins, needed, rounds_lost);
        return wins >= needed ? 1 : 2;
    }
    int out = peek_int(st, STATE_OUT_STAGE, -2);
    int place = out == -1 ? 1 : out >= 5 ? 2 : out == 4 ? 3 : 5;
    if (out == -2) return 0;
    LOG("battle result: rumble %s (knocked out in stage %d)",
        place == 1 ? "1st" : place == 2 ? "2nd" : place == 3 ? "3rd-4th" : "5th-8th", out);
    return place;
}

/* Tell the server: a plain frame on a connection of its own. */
static void report_result_ex(int mode, int place, int extra) {
    struct sockaddr_in a = { .sin_family = AF_INET, .sin_port = htons(GAME_PORT) };
    if (inet_pton(AF_INET, REDIRECT_TO, &a.sin_addr) != 1) {
        LOG("battle result: %s is not an IPv4 address, not reported", REDIRECT_TO);
        return;
    }
    int fd = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) return;
    struct timeval tv = { 2, 0 };
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof tv);
    uint8_t f[11] = { HOOK_BATTLE_RESULT >> 8, HOOK_BATTLE_RESULT & 0xff, 0, 0, 4, 0, 0,
                      (uint8_t)mode, (uint8_t)place, (uint8_t)(extra >> 8), (uint8_t)extra };
    int ok = connect(fd, (struct sockaddr *)&a, sizeof a) == 0 && write(fd, f, sizeof f) == sizeof f;
    close(fd);
    if (!ok) LOG("battle result: could not reach the server");
}

static void report_result(int mode, int place) { report_result_ex(mode, place, 0); }

/* Quest progress. The client counts its quests itself, battle ones too
 * (LogicQuest*EventHandler: knock outs, rounds, 3:0, rerolls, Clash,
 * upgrades), but the server's next home would put its own counts back. So
 * after a battle the hook sends the server the client's progress:
 *   home        = *(*(GameMode + 0x68) + 0x48)   (0x8223c4; commands 0x68da08)
 *   quests      = *(home + 0x180)                (0x975f88)
 *   active list = quests +0x30 (quest *[]), count +0x3c (0x59b4a4)
 *   quest       +0x38 id (0x3add64), +0x3c progress
 * frame: mode HOOK_QUEST_PROGRESS, count, then (id, progress) as big-endian
 * int32 pairs. */
#define HOOK_QUEST_PROGRESS 11         /* cmserver hookQuestProgress */
#define LOGIC_MODE_HOME     0x48
#define HOME_QUESTS         0x180
#define QUESTS_ACTIVE       0x30
#define QUESTS_ACTIVE_N     0x3c
#define QUEST_ID            0x38
#define QUEST_PROGRESS      0x3c
#define QUESTS_MAX          40

/* In a battle the game mode points to the battle, not the home: the quest
 * manager is remembered while at home (state 4) and read from there. */
static uint8_t *home_quests;

static uint8_t *quests_of(uint8_t *gm) {
    uint8_t *mode = peek_ptr(gm, GM_MODE);
    uint8_t *home = peek_ptr(peek_ptr(mode, LOGIC_BATTLE_OWNER), LOGIC_MODE_HOME);
    return peek_ptr(home, HOME_QUESTS);
}

static void remember_quests(uint8_t *gm) {
    uint8_t *qm = quests_of(gm);
    if (qm && qm != home_quests) {
        home_quests = qm;
        LOG("quests: manager at %p (%d active)", (void *)qm, peek_int(qm, QUESTS_ACTIVE_N, -1));
    }
}

/* The battle's own quests: at its start the client copies the home's
 * active quests that count in battle and have not expired (0x940898 ->
 * 0x4f6854, quest +0x1c end time > now) into a list per player, battle
 * +0x238 + 8 * player (0x32c3c4; a list: array +0x0, count +0xc). The
 * battle's events count on these copies (knock outs 0x5c59cc -> 0x4f28b0,
 * ...), so at the end their progress is the client's. */
#define BATTLE_QUESTS       0x238
#define LIST_ITEMS          0x0
#define LIST_N              0xc

static int battle_quests(uint8_t *b, uint8_t **list, int *count) {
    int n = peek_int(b, BATTLE_PLAYER_N, 0), me = -1;
    if (!b || n < 1 || n > 8) return 0;
    for (int i = 0; i < n && me < 0; i++)
        if (peek_int(peek_ptr(b, BATTLE_PLAYERS + i * 8), 0, -1) >= 0) me = i;
    int pick = -1;
    char line[160];
    int ll = 0;
    for (int i = 0; i < n; i++) {
        uint8_t *l = peek_ptr(b, BATTLE_QUESTS + i * 8);
        int c = l ? peek_int(l, LIST_N, -1) : -1;
        if (ll < (int)sizeof line - 16) ll += snprintf(line + ll, sizeof line - ll, " %d:%d", i, c);
        if (c > 0 && c <= 64 && peek_ptr(l, LIST_ITEMS) && (pick < 0 || i == me)) pick = i;
    }
    LOG("quests: battle lists (player: count)%s, ours %d", ll ? line : " none", me);
    if (pick < 0) return 0;
    uint8_t *l = peek_ptr(b, BATTLE_QUESTS + pick * 8);
    *list = peek_ptr(l, LIST_ITEMS);
    *count = peek_int(l, LIST_N, 0);
    return 1;
}

static void report_quests(uint8_t *gm, uint8_t *b) {
    uint8_t *list = NULL;
    int n = -1;
    if (!battle_quests(b, &list, &n)) {
        uint8_t *qm = quests_of(gm);
        if (!qm) qm = home_quests;
        list = peek_ptr(qm, QUESTS_ACTIVE);
        n = peek_int(qm, QUESTS_ACTIVE_N, -1);
    }
    if (!list || n < 0 || n > 64) { LOG("quests: not readable (%d)", n); return; }
    uint8_t f[7 + 2 + QUESTS_MAX * 8];
    int len = 2, k = 0;
    char line[256];
    int ll = 0;
    for (int i = 0; i < n && k < QUESTS_MAX; i++) {
        uint8_t *q = peek_ptr(list, i * 8);
        int32_t id = peek_int(q, QUEST_ID, -1), pr = peek_int(q, QUEST_PROGRESS, -1);
        if (id < 0 || pr < 0) continue;
        uint8_t *o = f + 7 + len;
        o[0] = id >> 24; o[1] = id >> 16; o[2] = id >> 8; o[3] = id;
        o[4] = pr >> 24; o[5] = pr >> 16; o[6] = pr >> 8; o[7] = pr;
        len += 8;
        k++;
        if (ll < (int)sizeof line - 24) ll += snprintf(line + ll, sizeof line - ll, " #%d %d", id, pr);
    }
    LOG("quests: client progress%s", k ? line : " (none)");
    f[0] = HOOK_BATTLE_RESULT >> 8; f[1] = HOOK_BATTLE_RESULT & 0xff;
    f[2] = len >> 16; f[3] = len >> 8; f[4] = len; f[5] = 0; f[6] = 0;
    f[7] = HOOK_QUEST_PROGRESS; f[8] = (uint8_t)k;
    struct sockaddr_in a = { .sin_family = AF_INET, .sin_port = htons(GAME_PORT) };
    if (inet_pton(AF_INET, REDIRECT_TO, &a.sin_addr) != 1) return;
    int fd = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) return;
    struct timeval tv = { 2, 0 };
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof tv);
    int ok = connect(fd, (struct sockaddr *)&a, sizeof a) == 0 && write(fd, f, 7 + len) == 7 + len;
    close(fd);
    if (!ok) LOG("quests: could not reach the server");
}

/* Diagnostics: a quest's unit (RequirementMini) is a reference into data
 * table 6 (0x6d3e64 / 0x60c69c, *(libg+0x1080778)): 69 items, the 55 of
 * characters.csv then the 14 of characters_heroes.csv. Logged once. */
#define LIBG_TABLE6 0x1080778
static void log_table6(void) {
    uint8_t *t = *(uint8_t **)G(LIBG_TABLE6);
    if (!t) { LOG("table 6: none"); return; }
    void **vt = *(void ***)t;
    int n = ((int (*)(void *))vt[0x30 / 8])(t);
    char line[256];
    int ll = 0;
    for (int i = 0; i < n && i < 40 && ll < (int)sizeof line - 24; i++) {
        uint8_t *it = ((uint8_t *(*)(void *, int))vt[0x38 / 8])(t, i);
        uint8_t *row = it ? *(uint8_t **)(it + 0x10) : NULL;
        ll += snprintf(line + ll, sizeof line - ll, " %d:%d", i, row ? *(int32_t *)(row + 8) : -1);
    }
    LOG("table 6: %d items, CSV rows%s", n, line);
}

/* Called every tick with the game state. */
static void watch_result(uint8_t *gm, int state) {
    static int reported, ended, seen, checked;
    if (!checked++) {
        uint8_t probe;
        if (peek(&probe, &probe, 1)) LOG("battle result: process_vm_readv not usable, results off");
    }
    if (state != 5) {
        if (!battle_mode) {
            /* not a bot battle of ours (the tutorial's): nothing to report */
        } else if (ended && !reported)
            LOG("battle result: end screen seen, but the battle was unreadable; not reported");
        else if (seen >= 300 && !reported) {
            LOG("battle result: left before the end (counts as the worst place)");
            report_result(battle_mode, 0);
        }
        if (seen) battle_mode = 0;
        reported = ended = seen = 0;
        return;
    }
    /* skip the first 3 s, when the root may still be the last battle's */
    if (++seen < 300 || reported) return;
    uint8_t *screen = *(uint8_t **)G(LIBG_BATTLE_ROOT);
    if (!peek_ptr(screen, SCREEN_END_SCREEN)) return;
    ended = 1;
    if (!battle_mode) return;
    int place = read_place(battle_mode, screen);
    if (!place) return;
    reported = 1;
    /* extra: 0x8000 | rounds lost (duel, 0x7f unknown) << 8 | rounds won */
    int extra = 0;
    if (rounds_won >= 0)
        extra = 0x8000 | ((rounds_lost >= 0 ? rounds_lost : 0x7f) & 0x7f) << 8 | (rounds_won & 0xff);
    report_result_ex(battle_mode, place, extra);
    report_quests(gm, peek_ptr(peek_ptr(peek_ptr(screen, SCREEN_LOGIC_OWNER), LOGIC_BATTLE_OWNER),
                               LOGIC_BATTLE));
}

/* Tutorial (a new player's, signalled by the server with mode 1). The
 * avatar is *(GameMain + 0x38) (0x5f6e6c -> 0x5b1868); its tutorial flags at
 * +0x130 are logged when they change (seen: 0x1 first battle, 0, 0x200 home
 * part; bit 0x100 = done, 0x5b073c). */
#define GM_AVATAR          0x38
#define AVATAR_TUTORIAL    0x130
#define HOOK_TUTORIAL_DONE 9           /* cmserver hookTutorialDone: hero chosen */

static void log_tutorial_flags(uint8_t *gm) {
    static int32_t last = -1;
    uint8_t *avatar = peek_ptr(gm, GM_AVATAR);
    int32_t flags = peek_int(avatar, AVATAR_TUTORIAL, -1);
    if (avatar && flags != last) {
        LOG("tutorial flags %#x", (unsigned)flags);
        last = flags;
    }
}

/* The hero is picked at the start of the first battle (ChooseStartingHeroHUD,
 * battle command LogicSelectStartingDeckCommand 52, execute 0x611fb8):
 *   player index = 0x7553fc(battle, command's avatar id)
 *   avatar = 0x603108(battle, index) = 0x4c98f0(*battle, index)
 *          = *(*battle + 0xc8 + index * 8)      <- battle avatars, not states
 *   0x545b0c(avatar, StartingChoiceIndex): avatar + 0xb8
 * starting_choices.csv: 1 Barbarian King, 2 Archer Queen, 3 Shield Maiden.
 * The avatar's id is at +0x24 (0x481834); the NPC's is negative. Every
 * player's id and value are logged when they change (a dump to check it). */
#define LEVEL_AVATARS         0xc8
#define AVATAR_ID             0x24
#define AVATAR_STARTING_CHOICE 0xb8
static int tutorial_choice;
static void read_starting_choice(uint8_t *gm) {
    static int32_t seen[8][2];
    uint8_t *b = peek_ptr(peek_ptr(peek_ptr(gm, GM_MODE), LOGIC_BATTLE_OWNER), LOGIC_BATTLE);
    uint8_t *level = peek_ptr(b, 0);
    int n = peek_int(b, BATTLE_PLAYER_N, 0);
    if (!level || n < 1 || n > 8) return;
    int changed = 0;
    char line[256];
    int len = 0;
    for (int i = 0; i < n; i++) {
        uint8_t *av = peek_ptr(level, LEVEL_AVATARS + i * 8);
        int32_t id = peek_int(av, AVATAR_ID, -9);
        int32_t c = peek_int(av, AVATAR_STARTING_CHOICE, -9);
        if (id >= 0 && c >= 1 && c <= 3) tutorial_choice = c;
        if (seen[i][0] != id || seen[i][1] != c) changed = 1;
        seen[i][0] = id;
        seen[i][1] = c;
        len += snprintf(line + len, sizeof line - len, " [avatar %d id %d: %d]", i, id, c);
        if (len >= (int)sizeof line) len = sizeof line - 1;
    }
    if (changed) LOG("tutorial: starting choice %d,%s", tutorial_choice, line);
}

static int chosen_hero_row(uint8_t *gm) {
    static const int rows[] = { 0, 12, 13, 15 };
    (void)gm;
    return tutorial_choice >= 1 && tutorial_choice <= 3 ? rows[tutorial_choice] : 0;
}
/* Reload without the player. After the tutorial battle the client is in
 * its offline home; the hook closes the socket, and the client's network
 * update (0x4f919c) shows the "Connection lost" popup: showError
 * 0x8ff948(gui, 4, 0, ...) stores the type at gui+0x124. Its button runs
 * the GUI's vtable+0x18 (0x6dd634, via the popup handler 0x6dee70, type 4
 * goes to the default case 0x6defb8), which sets gui+0x193; the GUI's
 * update (0x6246b0) then reloads the game (0x9b20f0), and that clears the
 * popup (0x7dbe00) and +0x124. gui = *(Ghidra 0x1198a28), getter 0x791c98;
 * its vtable is Ghidra 0xec6f00 (file 0xdc5f00: the data segment is
 * mapped 0x1000 above its file offset).
 * So as soon as gui+0x124 is 4, the hook does what the button does. */
#define LIBG_GUI            0x1098a28
#define LIBG_GUI_VTABLE     0xdc6f00   /* Ghidra 0xec6f00 */
#define GUI_ERROR_TYPE      0x124
#define GUI_RELOAD          0x193
#define GUI_VT_RELOAD       0x18
#define ERROR_CONNECTION_LOST 4
static int reload_ms = -1;
static void watch_reload(void) {
    if (reload_ms < 0) return;
    reload_ms += 10;
    uint8_t *gui = *(uint8_t **)G(LIBG_GUI);
    int type = peek_int(gui, GUI_ERROR_TYPE, -1);
    if (type == ERROR_CONNECTION_LOST) {
        void **vt = (void **)peek_ptr(gui, 0);
        if (vt == G(LIBG_GUI_VTABLE)) {
            ((void (*)(void *))vt[GUI_VT_RELOAD / 8])(gui);
            LOG("reload: \"connection lost\" after %d ms, pressed its button (vtable+0x18)", reload_ms);
        } else {
            gui[GUI_RELOAD] = 1;
            LOG("reload: \"connection lost\" after %d ms, gui vtable %p unexpected, set gui+0x193", reload_ms, (void *)vt);
        }
        reload_ms = -1;
    } else if (reload_ms > 10000) {
        LOG("reload: no \"connection lost\" within 10 s (error type %d), the player has to reload", type);
        reload_ms = -1;
    }
}

/* The tutorial's first battle needs the offline flag all through (turning
 * it off mid-battle crashed the client). When its end screen appears the
 * battle is over: report the hero picked at its start and turn the flag
 * off, so leaving the end screen goes home online (GoHome) and gets the
 * server's home with the hero. If the battle ends without an end screen,
 * reconnect instead, and press the "Connection lost" popup's reload
 * button for the player (watch_reload). */
#ifdef CMOFFLINE_SECTOR_DUMP
/* Mini Room work (docs/MINIROOM.md): write the running bot battle as the
 * client's own SectorState, the way its offline layer answers
 * RequestSectorState (0x876d18), and log the bytes. Runs on this thread,
 * not the game's, once per battle 2 s in (a test build only). */
#define GH(a) G((a) - GHIDRA_BASE)
static void dump_sector_state(void) {
    void *(*op_new)(size_t) = GH(0x2728e0);
    void (*msg_ctor)(void *) = GH(0x346be4);          /* SectorStateMessage */
    uint8_t *(*msg_stream)(void *) = GH(0x9a6a98);
    void (*reset_bits)(void *) = GH(0x788294);
    void (*battle_encode)(void *, void *, int, void *, int) = GH(0x9076c4);
    void (*battle_encode_tail)(void *, void *, int, int) = GH(0x80e1b8);
    uint8_t *(*game_mode)(void) = GH(0x5952bc);

    uint8_t *mode = game_mode();
    uint8_t *battle = mode ? *(uint8_t **)(mode + 0x68) : NULL;
    if (!battle) { LOG("sector dump: no battle"); return; }
    void *msg = op_new(0x88);
    msg_ctor(msg);
    uint8_t *s = msg_stream(msg);
    void **vt = *(void ***)s;
    ((void (*)(void *, int))vt[0x70 / 8])(s, 0);      /* writeByte 0 */
    reset_bits(s);
    ((void (*)(void *, int))vt[0x40 / 8])(s, 1);      /* writeBoolean: full */
    battle_encode(battle, s, 1, NULL, 0);
    int mid = *(int32_t *)(s + 0x14);
    battle_encode_tail(battle, s, 1, 0);
    int len = *(int32_t *)(s + 0x14);
    const uint8_t *buf = *(uint8_t **)(s + 0x20);
    LOG("sector dump: %d bytes (battle part ends at %d)", len, mid);
    static const char hx[] = "0123456789abcdef";
    char line[1100];
    for (int off = 0; off < len; off += 512) {
        int n = len - off < 512 ? len - off : 512, l = 0;
        for (int i = 0; i < n; i++) {
            line[l++] = hx[buf[off + i] >> 4];
            line[l++] = hx[buf[off + i] & 15];
        }
        line[l] = 0;
        LOG("sector %05d %s", off, line);
    }
}

static void watch_sector_dump(int state) {
    static int ms, done;
    if (state != 5 || !battle_mode) { ms = done = 0; return; }
    if (done || (ms += 10) < 2000) return;
    done = 1;
    dump_sector_state();
}
#endif

static void watch_tutorial(uint8_t *gm, int state) {
    static int battle_ms, finished;
    if (!tutorial) return;
    volatile uint8_t *offline = G(LIBG_OFFLINE_FLAG);
    if (state == 5) {
        battle_ms += 10;
        read_starting_choice(gm);
        uint8_t *root = *(uint8_t **)G(LIBG_BATTLE_ROOT);
        if (!finished && battle_ms > 3000 && peek_ptr(root, SCREEN_END_SCREEN)) {
            finished = 1;
            report_result(HOOK_TUTORIAL_DONE, chosen_hero_row(gm));
            *offline = 0;
            LOG("tutorial battle over: hero row %d reported, back online", chosen_hero_row(gm));
        }
        if (!finished) *offline = 1;
        return;
    }
    if (battle_ms > 3000) {
        if (!finished) {
            report_result(HOOK_TUTORIAL_DONE, chosen_hero_row(gm));
            *offline = 0;
            int fd = game_fd;
            LOG("tutorial battle over without end screen: hero row %d reported, reconnecting (fd %d)",
                chosen_hero_row(gm), fd);
            if (fd >= 0) {
                shutdown(fd, SHUT_RDWR);
                reload_ms = 0;
            }
        }
        tutorial = 0;
        battle_ms = finished = 0;
        return;
    }
    *offline = 1; /* until the battle starts */
}

/* LogicUpdateChronosEventsCommand 417 (execute 0x682b80, vtable Ghidra
 * 0xe98070 slot +0x18) replaces the events; then 0x7b5e50(home, avatar)
 * links the season manager (home+0x1c8) to the new events and hands out
 * the quests of new events (a new mini's chain) - but only with the
 * offline flag set (0x3b4300). Online it returns at once, the season
 * manager keeps the freed season object, and the client crashed (0099).
 * So after the command ran, call 0x7b5e50 once more with the flag set
 * for that call only (inside 0x682b80 the flag also changes 0x6b7fe0,
 * which is left alone). The server sends 417 when a mini is unlocked. */
#define LIBG_CHRONOS_VTABLE  0xd98070   /* Ghidra 0xe98070 */
#define CHRONOS_VT_EXECUTE   0x18
#define LIBG_CHRONOS_EXECUTE 0x582b80   /* Ghidra 0x682b80 */
#define LIBG_EVENTS_RELINK   0x6b5e50   /* Ghidra 0x7b5e50 */
#define MODE_HOME            0x48       /* 0x8223c4 */
#define MODE_AVATAR          0x50       /* 0x4eea2c */
/* The events: home+0x140 the events manager (0x51e094), its +0x10 the
 * collection (0x6b7fe0 frees the old one, then takes the new); the
 * command's own collection is +0x20, freed with the command (0x4ea4ac).
 * With only the season manager linked anew the client still crashed 4 s
 * after a 417 (0117: in "[LIVE EVENT] onEventVisible", 0x296ca0, copying
 * an event's name from freed memory): more holds on to those events. So
 * neither collection is freed: the old one is taken out before the
 * command runs, the command's own after it (its encoder writes "no
 * events" then, 0x3bf12c). A few kilobytes per new mini, kept. */
#define HOME_EVENTS          0x140
#define EVENTS_COLLECTION    0x10
#define CHRONOS_CMD_EVENTS   0x20
static int (*real_chronos_execute)(void *cmd, void *mode);
static int cm_chronos_execute(void *cmd, void *mode) {
    void *home = peek_ptr(mode, MODE_HOME), *avatar = peek_ptr(mode, MODE_AVATAR);
    uint8_t *events = home ? peek_ptr(home, HOME_EVENTS) : NULL;
    void *old = events ? *(void **)(events + EVENTS_COLLECTION) : NULL;
    if (old) *(void **)(events + EVENTS_COLLECTION) = NULL; /* not freed */
    int r = real_chronos_execute(cmd, mode);
    if (events && !*(void **)(events + EVENTS_COLLECTION))
        *(void **)(events + EVENTS_COLLECTION) = old; /* nothing new taken */
    else if (cmd)
        *(void **)((uint8_t *)cmd + CHRONOS_CMD_EVENTS) = NULL; /* not freed */
    if (r == 0 && home && avatar) {
        volatile uint8_t *offline = G(LIBG_OFFLINE_FLAG);
        uint8_t was = *offline;
        *offline = 1;
        ((void (*)(void *, void *))G(LIBG_EVENTS_RELINK))(home, avatar);
        *offline = was;
        LOG("events update (417): season manager linked to the new events, new quests handed out");
    } else {
        LOG("events update (417): result %d, not linked", r);
    }
    return r;
}

static void hook_chronos(void) {
    void **slot = (void **)G(LIBG_CHRONOS_VTABLE + CHRONOS_VT_EXECUTE);
    if (*slot != G(LIBG_CHRONOS_EXECUTE)) {
        LOG("417 vtable slot %p unexpected, events update not hooked", *slot);
        return;
    }
    real_chronos_execute = (int (*)(void *, void *))*slot;
    if (patch_pointer(slot, (void *)cm_chronos_execute)) {
        LOG("417 vtable: mprotect failed");
        return;
    }
    LOG("events update (417) hooked");
}

static void battles_main(void) {
    int bad = 0;
    bad |= hook_got(LIBG_GOT_READ,     "read",     (void *)cm_read,     (void **)&real_read);
    bad |= hook_got(LIBG_GOT_RECVFROM, "recvfrom", (void *)cm_recvfrom, (void **)&real_recvfrom);
    bad |= hook_got(LIBG_GOT_WRITE,    "write",    (void *)cm_write,    (void **)&real_write);
    bad |= hook_got(LIBG_GOT_SENDTO,   "sendto",   (void *)cm_sendto,   (void **)&real_sendto);
    bad |= hook_got(LIBG_GOT_CONNECT,  "connect",  (void *)cm_connect,  (void **)&real_connect);
    bad |= hook_got(LIBG_GOT_CLOSE,    "close",    (void *)cm_close,    (void **)&real_close);
    if (bad) { LOG("battle hooks incomplete, bot battles off"); return; }
    LOG("bot battles ready");

    volatile uint8_t *offline = G(LIBG_OFFLINE_FLAG);
    int in_battle_ms = -1;  /* >=0 from queuing a battle until we go back online */
    int battle_ms = 0;      /* time spent in state 5 */
    int hold = 0, seen_battle = 0, wait_ms = 0, last_state = -1;
    for (;;) {
        usleep(10000);
        /* The live server switched renaming on; globals.csv has no
         * ENABLE_NAME_CHANGE, so the button stays disabled. Globals can be
         * reloaded, so keep it set. */
        uint8_t *glb = *(uint8_t **)G(LIBG_GLOBALS);
        if (glb && !glb[GLB_NAME_CHANGE]) {
            glb[GLB_NAME_CHANGE] = 1;
            LOG("name change enabled");
        }
        watch_reload();
        uint8_t *gm = *(uint8_t **)G(LIBG_GAME_MAIN);
        if (!gm) continue;
        int state = *(volatile int32_t *)(gm + GM_STATE);
        if (state != last_state) {
            LOG("game state %d -> %d (offline flag %d)", last_state, state, *offline);
            last_state = state;
            /* not at load: the client crashed during its start-up
             * (0x60a330 -> 0x3e4eb0 jumped into nowhere) with the vtable
             * patched that early; in the home it is past that */
            static int chronos_hooked;
            if (state == 4 && !chronos_hooked) {
                chronos_hooked = 1;
                hook_chronos();
            }
        }
        watch_result(gm, state);
#ifdef CMOFFLINE_SECTOR_DUMP
        watch_sector_dump(state);
#endif
        {
            static int table6_logged;
            if (state == 4 && !table6_logged++) log_table6();
            if (state == 4) remember_quests(gm);
        }
        watch_tutorial(gm, state);
        log_tutorial_flags(gm);

        if (pending_mode > 0 && in_battle_ms < 0) {
            uint8_t *mode = (state == 4 || state == 5) ? *(uint8_t **)(gm + GM_MODE) : NULL;
            if (!mode) {
                if ((wait_ms += 10) > 5000) { LOG("no home mode, dropping signal"); pending_mode = 0; wait_ms = 0; }
                continue;
            }
            choose_bot(pending_bot);
            choose_board(pending_board);
            battle_mode = pending_mode;
            level_bot_decks(pending_hero_level, pending_mini_level);
            *offline = 1;
            *(volatile int32_t *)(mode + MODE_MATCH) = pending_mode;
            *(volatile uint8_t *)(mode + MODE_MATCH_FLAG) = 0;
            hold = pending_hold;
            LOG("offline battle queued (mode %d, %s)", pending_mode,
                hold ? "offline for the whole battle" : "online after 1 s");
            pending_mode = 0;
            wait_ms = 0;
            in_battle_ms = 0;
            battle_ms = 0;
            seen_battle = 0;
            continue;
        }
        if (in_battle_ms < 0) continue;
        in_battle_ms += 10;
        if (state == 5) { battle_ms += 10; seen_battle = 1; } else battle_ms = 0;

        if (!hold) {
            if (battle_ms >= 1000 || in_battle_ms > 15000) {
                *offline = 0;
                LOG(battle_ms >= 1000 ? "battle running, back online" : "battle never started, back online");
                in_battle_ms = -1;
            }
            continue;
        }
        /* hold: the battle runs fully offline. When the client is back at the
         * home (state 4 after 5) it shows its local offline home, so go online
         * and drop the connection: the client reconnects, logs in again and
         * gets the server's home. */
        if ((seen_battle && state == 4) || (!seen_battle && in_battle_ms > 15000)) {
            *offline = 0;
            int fd = game_fd;
            LOG(seen_battle ? "battle over, back online, reconnecting (fd %d)" : "battle never started, back online (fd %d)", fd);
            if (seen_battle && fd >= 0) shutdown(fd, SHUT_RDWR);
            in_battle_ms = -1;
        }
    }
}
#endif /* CMOFFLINE_NO_BATTLES */

#ifdef CMOFFLINE_LOCAL_SERVER
#ifndef SERVER_KEY_HEX
#error "CMOFFLINE_LOCAL_SERVER needs -DSERVER_KEY_HEX='\"<64 hex chars>\"'"
#endif
/* Options for the in-game server, e.g. "progression=fresh" (build.sh
 * PROGRESSION=...; cmserver -progression). */
#ifndef CMSERVER_OPTIONS
#define CMSERVER_OPTIONS ""
#endif
/* The app's own files folder: /data/user/<user>/<package>/files. */
static int app_files_dir(char *out, size_t n) {
    char pkg[128] = {0};
    FILE *f = fopen("/proc/self/cmdline", "r");
    if (!f) return -1;
    size_t got = fread(pkg, 1, sizeof pkg - 1, f);
    fclose(f);
    if (!got || !pkg[0]) return -1;
    char *colon = strchr(pkg, ':');        /* "pkg:process" */
    if (colon) *colon = 0;
    snprintf(out, n, "/data/user/%u/%s/files", (unsigned)(getuid() / 100000), pkg);
    mkdir(out, 0700);
    return 0;
}

/* Start cmserver (cmserver/cmd/libcmserver) inside the game process. */
static void start_local_server(void) {
    char dir[256];
    if (app_files_dir(dir, sizeof dir)) { LOG("local server: no files dir"); return; }
    void *h = dlopen("libcmserver.so", RTLD_NOW);
    if (!h) { LOG("local server: %s", dlerror()); return; }
    int (*start)(const char *, const char *, const char *) =
        (int (*)(const char *, const char *, const char *))dlsym(h, "CmServerStart");
    if (!start) { LOG("local server: CmServerStart missing"); return; }
    int r = start(dir, SERVER_KEY_HEX, CMSERVER_OPTIONS);
    LOG(r == 0 ? "local server on 127.0.0.1:9339, data in %s" : "local server failed (%s), see logcat -s cmserver", dir);
}
#endif

static void *cmoffline_main(void *arg) {
    (void)arg;
#ifdef CMOFFLINE_LOCAL_SERVER
    start_local_server();
#endif

    for (int i = 0; i < 12000 && !libg_base; i++) {
        dl_iterate_phdr(find_libg, NULL);
        if (!libg_base) usleep(5000);
    }
    if (!libg_base) { LOG("libg.so never loaded"); return NULL; }

    if (strcmp(libg_build_id, LIBG_BUILD_ID) != 0) {
        LOG("unsupported libg.so (%s)", libg_build_id[0] ? libg_build_id : "none");
        return NULL;
    }
    LOG("libg.so 1.1143.2 at %p", (void *)libg_base);

    real_getaddrinfo = (getaddrinfo_fn)dlsym(RTLD_DEFAULT, "getaddrinfo");
    void **slot = G(LIBG_GOT_GETADDRINFO);
    for (int i = 0; i < 2000 && *slot != (void *)real_getaddrinfo; i++) usleep(1000);
    if (*slot == (void *)real_getaddrinfo && patch_pointer(slot, (void *)cm_getaddrinfo) == 0)
        LOG("%s now resolves to %s", GAME_HOST, REDIRECT_TO);
    else
        LOG("could not install the redirect");

#ifdef CMOFFLINE_FORCE_OFFLINE
    volatile uint8_t *offline = G(LIBG_OFFLINE_FLAG);
    LOG("offline mode on");
    for (;;) {
        if (*offline != 1) *offline = 1;
        usleep(1000);
    }
#elif !defined(CMOFFLINE_NO_BATTLES)
    battles_main();
#endif
    return NULL;
}

__attribute__((constructor))
static void cmoffline_init(void) {
    LOG("loaded");
    install_crash_logger();
    pthread_t t;
    if (pthread_create(&t, NULL, cmoffline_main, NULL) == 0) pthread_detach(t);
}
