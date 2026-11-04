Clash Mini 1.1143.2 — Handshake Crypto RE Notes
Overview

Initial investigation into the Clash Mini 1.1143.2 login handshake, focusing on the client-side crypto routines in libg.so and the handshake flow needed for a clean implementation in cmserver.

The main goal is to understand how the client establishes its session crypto state, how the Login packet is encrypted, and what the server needs to send during the handshake for the client to proceed.

Most of the work so far has involved static analysis of the native libraries, tracing crypto-related functions, and checking the behavior against the client. The underlying primitives are mostly identified, but the exact key derivation path is still unresolved.

Offsets below refer to file or virtual addresses in the relevant library unless otherwise noted.

1. Handshake flow

The login sequence observed so far is:

ClientHello (10100) → ServerHello (20100) → Login (10101)

The client waits for a ServerHello response before sending its Login packet. Structurally, a length-prefixed byte array is enough to get the client to continue, but that doesn't necessarily mean the contents are valid or sufficient for completing the crypto handshake.

This distinction is important because the client can move on to Login while still ending up with a packet that the server cannot decrypt.

2. Crypto primitives and constants

The relevant routines appear to use the following primitives:

X25519 for Diffie–Hellman key exchange.
XSalsa20-Poly1305 for authenticated encryption through the secretbox interface.
BLAKE2b for deriving values used by the session crypto.

The crypto-related dependencies identified during analysis include:

curve25519-dalek 4.1.3
crypto_secretbox 0.1.1
blake2 0.10.6
Client-side constants

A 32-byte key-like constant was found in libg.so at .rodata file offset 0xbc3de0.

This appears to be relevant to the static-key side of the handshake. The exact role of the value still needs to be confirmed by tracing where it is read and how it feeds into the key exchange.

The XSalsa20 sigma constants are also present in the same general read-only data region, at approximately 0xbc3840. They are stored as spaced 32-bit words rather than as a contiguous ASCII string, which explains why a straightforward string search doesn't find them.

Other useful locations in libg.so:

BLAKE2b IV table: 0xbc0c58
BLAKE2b initialization routine: 0x6c8f6c
Secretbox/Salsa-related routines: 0x211d44, 0x431240, 0x6d35dc

These are starting points for further tracing rather than confirmed entry points for the Login encryption path.

3. Login packet decryption

The Login decryption path has been narrowed down to the following routines:

open_login: 0x67350
Caller of open_login: 0x95df8
Decryption routine: 0x924e4
Wrapper around the decryption routine: 0x8f030

The decryption function takes its arguments in the following order:

key = x0, nonce = x1, associated_data = x2, ciphertext_and_tag = x3

The implementation matches XSalsa20-Poly1305 secretbox behavior.

Key handling

The key passed into open_login is read from a 32-byte field at session + 0x21.

It isn't derived directly inside open_login. Instead, the function receives a session structure that already contains the relevant crypto state.

The caller copies a 176-byte block from the connection object, starting at offset 0x69:

x20[0x69:0x119]

This suggests that the session key and related fields are populated earlier in the handshake, before Login processing begins.

The next step is to trace where those fields are first written rather than spending more time inside the decryption routine itself.

Nonce derivation

The nonce passed to secretbox appears to be a 24-byte BLAKE2b output.

The input is assembled from three components:

session[0x41:0x59] — 24 bytes
session[0x01:0x21] — 32 bytes
T — a 32-byte value produced by transforming data beginning at session + 0x79

The transformation appears to pass through 0xa3dd4 and 0xa4b78.

In simplified form:

nonce = BLAKE2b-24(field_A || field_B || T)

The overall structure is reasonably clear, but the exact byte-level representation of the fields still needs to be confirmed against the values used by the client.

A separate BLAKE2b-32 operation also appears at 0x91b78. Its purpose hasn't been established yet. It may be involved in another session value or a reply-side key, but that remains a hypothesis.

4. X25519 and session setup

The X25519 scalar multiplication routine appears at 0xa3de4, with the ladder implementation around 0xa49d8.

The scalar-clamping logic is visible in the native code, including the low-byte mask 0xf8 and the final-byte operation equivalent to:

byte31 = 0x40 | (b & 0x3f)

One complication is that the scalar multiplication routine is called indirectly through a function pointer or vtable. There is no straightforward direct bl reference to follow from the current call site.

This makes the static call graph harder to reconstruct and is currently one of the main blockers.

The random-number generation PLT stub is at 0x142760. Known callers include:

0x11eff0
0x11f060
0x11f148
0x11f1a0

These sites are worth checking for key generation, but there is no clear connection to the X25519 routine yet. They may be used for unrelated identifiers, tokens, or other random session values.

5. Questions still open
5.1 Where does the session key come from?

The 32-byte field at session + 0x21 is the key passed to Login decryption. It is likely derived from a Diffie–Hellman exchange, but the exact inputs and derivation steps haven't been identified.

Possibilities worth investigating include:

X25519 using a static private key and the client's public key.
X25519 using an ephemeral server key and a client-provided public key.
A more involved exchange in which the static key authenticates or protects ephemeral key material.

At this stage, none of these should be treated as confirmed.

5.2 What does ServerHello actually contain?

The current tests show that the client proceeds after receiving a structurally valid ServerHello, but the crypto significance of its body is still unknown.

One possibility is that the response contains an ephemeral public key used later in the key exchange. If so, the server would need to generate the corresponding key pair and preserve the private key for the session.

This would also explain why a response that satisfies the packet parser can still lead to a Login decryption failure.

The next useful step is to inspect the client's handling of the ServerHello body and identify which fields are stored in the connection object.

5.3 What are the exact nonce inputs?

The BLAKE2b input structure has been narrowed down, but the precise byte layout still needs to be verified.

In particular, the transformed value T needs to be traced back to its source, and the session fields need to be compared with their original wire-format values.

6. Current implementation status

The initial implementation in cmserver has the secretbox decryption primitive in place.

Several candidate BLAKE2b input combinations have also been tested using values such as:

Session nonce
Client public key
Server public key
DH shared secret
Inline Login nonce

None of the combinations tested so far produce a matching decryption result.

That points to a missing or incorrectly derived input rather than an obvious problem with the secretbox primitive itself. The ServerHello/key-exchange path is one area that still needs closer inspection.

7. Next steps

The most productive next step is to stop guessing at the key derivation and observe the client at the point where it encrypts the Login packet.

A controlled runtime trace should help establish:

The exact 32-byte key passed to the encryption routine.
The 24-byte nonce used for that packet.
The plaintext immediately before encryption.
The call path that produces those values.

The likely starting points are the BLAKE2b initialization routine at 0x6c8f6c and the Salsa/secretbox-related functions identified earlier. These locations need to be narrowed down to the actual Login path before drawing conclusions from their inputs.

If runtime tracing isn't available, the alternative is to continue following the indirect X25519 call and trace writes to the session crypto fields until their origin is clear.

8. libg.so tracing reference

The following locations are useful for continuing the static investigation:

Item	Offset
BLAKE2b IV table	0xbc0c58
XSalsa20 sigma constants	0xbc3840
32-byte key-like constant	0xbc3de0
BLAKE2b initialization	0x6c8f6c
Secretbox/Salsa routine	0x211d44
Secretbox/Salsa routine	0x431240
Secretbox/Salsa routine	0x6d35dc
BLAKE2b-init callers	0x4b85f4, 0x636e90, 0x72d438

The offsets refer to the libg.so build with ID:

128b8fb1640843833d20eb35438f69decd98f8df

For runtime analysis, account for the module's load base when translating file or virtual offsets into process addresses. The exact adjustment depends on the ELF mapping and the type of offset being used.

Summary

The basic handshake sequence and Login decryption primitive are understood. The remaining issue is identifying how the session key is established and how the nonce inputs are populated before the client sends Login.

The strongest lead is the earlier handshake stage that fills the connection's crypto state. Tracing that stage should clarify whether the missing piece is an ephemeral key exchange, a static-key operation, or an additional derivation step.

Until those inputs are confirmed, further brute-force testing of BLAKE2b combinations is unlikely to be as useful as tracing the actual key-exchange and Login-encryption paths.
