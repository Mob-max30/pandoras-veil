================================================================================
                              PANDORA'S VEIL
                  DEVICE-BOUND ZERO-KNOWLEDGE SECRET RELAY
                    FINAL MASTER README — 36-HOUR BUILD
================================================================================
REPO:    https://github.com/Mob-max30/pandoras-veil
TEAM:    Pranav (Team Leader / Crypto Core / Integration)
         Pavan  (Backend Relay Lead)
         Ujwal  (CLI/TUI + Demo Lead)
BRANCHES: main, dev, feature-crypto-core, feature-backend-relay,
          feature-tui-sandbox
TIME:    36 hours, fixed. This document is FINAL — do not rewrite it again
         during the build. Edit only to fix something now factually wrong.
================================================================================

READ THIS FIRST IF YOU ARE AN LLM OR TEAMMATE PICKING THIS UP
-----------------------------------------------------------------
This file is the single, authoritative context document for Pandora's Veil.
It supersedes every earlier draft (browser Web Crypto version, Bash+age CLI
version, hand-rolled Go crypto version). Do not resurrect any of those.

The three of you should each be able to read this file once and know:
what this is, why it matters, what MUST work, what you personally own,
when you must stop and sync with a teammate, and what to build first.

Non-negotiable ground rules, all of which come from lessons already learned
in earlier planning rounds — do not relitigate any of these:

1. CRYPTOGRAPHY IS A LIBRARY CALL, NOT A PROTOCOL YOU DESIGN.
   Use `filippo.io/age` (Go implementation of the `age` encryption tool) to
   do all encryption/decryption. Do NOT hand-implement X25519 ECDH + HKDF +
   AES-GCM composition yourselves. Every primitive involved is
   established, but the *composition* of those primitives is exactly where
   real bugs live under time pressure, and `age` has already done that
   composition correctly and had it reviewed. This is closed — see section
   6 for why and exactly how to use it.

2. NO CUSTOM CRYPTOGRAPHY, EVER. No inventing an algorithm, no "PGP-like"
   scheme built from scratch. `age` already IS the modern answer to that
   idea.

3. THE BROWSER/WASM DEMO SANDBOX IS CUT FOR THIS BUILD. Not deprioritized —
   cut. The live demo is two real terminals side by side. This removes an
   entire category of engineering risk (Go→WASM compilation, JS bridge,
   xterm.js) for a component whose only job was making the demo watchable,
   which two terminals already do. If there is genuinely time left after
   everything in section 8 is done and rehearsed, it can be reconsidered —
   but plan as if it does not exist.

4. BUILD A PLAIN, BORING CLI FIRST. `pandora send`, `pandora read`,
   `pandora init`, `pandora identity` — flag-based, stdin/stdout, no TUI
   framework required to function. Bubble Tea (interactive TUI) is a layer
   Ujwal adds ON TOP of the working plain CLI, never a replacement for it.
   If Bubble Tea isn't finished or breaks near the deadline, the demo falls
   back to the plain CLI and loses nothing except visual polish.

5. NO MORE PLANNING DOCUMENTS. This file is it. Every hour spent writing
   more context is an hour not spent building or rehearsing. Documentation
   is worth the least of any rubric category (10 of 100 marks) and you are
   already far ahead on it.

6. THE HARD CHECKPOINT IS HOUR 12. See section 12. Read it now so you know
   what gets cut if you're behind.


================================================================================
1. ONE-SENTENCE PRODUCT DEFINITION
================================================================================
Pandora's Veil is a native, device-bound secret-sharing system where:

    THE SHARE LINK LOCATES THE SECRET,
    BUT THE AUTHORIZED DEVICE KEY AUTHORIZES DECRYPTION.

Even if the share link is leaked publicly, an unauthorized device cannot
decrypt the secret.

North star line for the demo and any pitch material:

    "Even if the link leaks, the secret doesn't."


================================================================================
2. THE PROBLEM WE ARE SOLVING
================================================================================
CloneFest's problem statement asks for a modern reinterpretation of the
problem PrivateBin addresses: secure, controlled sharing of sensitive text.
PrivateBin's approach (client-side browser encryption, password-derived
link) has two structural weaknesses we are specifically targeting:

PROBLEM A — BROWSER TRUST IS RE-ESTABLISHED ON EVERY VISIT.
Browser-delivered JavaScript encryption depends on the server serving
correct, unmodified code every single time a page loads. A compromised
server can quietly serve a modified encryptor to the next visitor. We move
the cryptographic trust boundary out of the browser into a native,
installed-once Go client, so trust isn't re-established per visit.

PROBLEM B — A LINK (PLUS PASSWORD) IS THE ONLY THING GUARDING THE SECRET.
    Sender encrypts secret -> Shareable URL -> URL accidentally posted
    publicly -> Anyone holding the URL (and any embedded/shared password)
    can attempt access.
This is true of PrivateBin and virtually every pastebin-style tool. We
change the model:
    Sender encrypts FOR a specific recipient device -> Shareable URL ->
    URL leaks -> Authorized device: decrypts. Unauthorized device: rejected.
Because decryption requires the recipient's private key, which never
leaves their device and is never derivable from the link, a leaked link is
provably useless to anyone else.


================================================================================
3. WHAT THIS IS AND ISN'T (say this correctly if a judge asks)
================================================================================
This is NOT "PrivateBin with a prettier UI." This is NOT "a new encryption
algorithm." This IS a device-bound secret-sharing architecture built on
established cryptographic primitives (via `age`) with a native CLI/TUI
client.

HONEST FRAMING FOR JUDGES — GET THIS RIGHT, DO NOT OVERCLAIM:
Encrypting to a recipient's public key is not new cryptography — it's what
PGP has done for decades. If asked "isn't this just GPG?", the correct
answer is: "We're not claiming new cryptography. We're claiming that
quick, disposable secret-sharing tools conventionally avoid the PGP model
specifically because of key-exchange friction — and we removed that
friction (automatic device identity, fingerprint exchange folded directly
into the send flow) while keeping the security property PGP has and
PrivateBin doesn't." That answer demonstrates you understand your own
prior art, which is worth more to a judge than pretending the crypto is
novel.

THE ACTUALLY SHARPER CLAIM, LEAD WITH THIS ONE:
The mandatory, hard-stop fingerprint verification before encryption is
less common than public-key encryption itself. Most link-sharing tools —
even ones using public-key crypto — don't force the sender to independently
verify they're encrypting to the correct key before proceeding. This one
UX/security design decision is what closes the "malicious relay swaps the
public key" attack, and it's the detail worth leading a pitch with.


================================================================================
4. SECURITY PHILOSOPHY
================================================================================
1. ENCRYPT LOCALLY — plaintext enters the native client; the backend never
   receives plaintext.
2. PRIVATE KEYS STAY LOCAL — never uploaded, never transmitted, ever.
3. ESTABLISHED CRYPTOGRAPHY — via the `age` library; no custom algorithms,
   no hand-composed protocol.
4. VERIFY RECIPIENT IDENTITY — mandatory out-of-band fingerprint check
   before every encryption.
5. MINIMIZE SERVER TRUST — the server stores encrypted material and the
   minimum metadata needed for routing/lifecycle, nothing else.
6. BE HONEST ABOUT METADATA — the claim is "zero knowledge of payload
   content," not "zero knowledge of all metadata." The server can observe
   timestamps, TTL, ciphertext size, IP/routing info, and the recipient
   handle. Say this plainly wherever the security model is described.


================================================================================
5. THREAT MODEL
================================================================================
WE ARE MITIGATING:
A. Compromised/untrusted browser delivery — production crypto runs in the
   compiled Go client, not browser JS.
B. Malicious/compromised relay attempting key substitution — mandatory
   fingerprint verification (section 3/9) catches this before encryption.
C. Link leakage — possession of the URL alone does not grant decryption.
D. Storage exposure — a leaked database/Redis dump yields ciphertext and
   metadata, never plaintext or private keys.

WE ARE NOT CLAIMING TO SOLVE (state this in the demo/docs, don't hide it):
- the sender's machine being compromised before encryption;
- the recipient's machine being compromised during/after decryption;
- a stolen recipient private key;
- an attacker with OS-level control of either device;
- a user voluntarily giving away their private key;
- undiscovered implementation bugs;
- a compromised underlying crypto library.
This is a hackathon prototype using established cryptography, not a
formally audited production system. Say so.

ONE MORE HONEST LINE WORTH ADDING: if a recipient's static private key is
ever compromised, every secret ever sent to that identity becomes
retroactively decryptable, because the sender's ephemeral public key
travels alongside the ciphertext. This is normal for a static-recipient
public-key scheme (not a flaw introduced by this design), not full forward
secrecy — don't claim forward secrecy in any pitch material.


================================================================================
6. CRYPTOGRAPHIC ARCHITECTURE (CLOSED DECISION)
================================================================================
Library: `filippo.io/age` (Go). Do not reimplement its internals.

WHY: age's X25519 recipient mode already performs exactly the flow we want
— ephemeral X25519 key agreement, HKDF-based key derivation, ChaCha20-
Poly1305 (age's default AEAD) content encryption — audited, reviewed,
widely used. Hand-composing this ourselves under a 36-hour deadline is the
single highest-risk decision available to this project; using the library
removes that risk almost entirely and takes under an hour to wire up.

WHAT THIS SIMPLIFIES: because `age` handles the envelope format
internally, we do NOT need to hand-design a payload JSON structure with
separate wrapped_content_key/wrap_nonce/payload_nonce fields. The
"payload" the server stores is just the raw bytes `age.Encrypt(...)`
produces, plus lifecycle metadata. This is a real reduction in surface
area — fewer fields to get wrong, less to test.

7.1 DEVICE INITIALIZATION — `pandora init`
    - generates an X25519 keypair via `age.GenerateX25519Identity()`;
    - stores the identity (private key) locally, e.g. `~/.pandora/identity`
      — file permissions restricted (0600) immediately after write;
    - the public key (`identity.Recipient().String()`, an age1... string)
      is what gets registered with the relay under a human-readable handle
      (e.g. `PV-A8F4-92KD`).

7.2 DEVICE FINGERPRINT
    - fingerprint = short, deterministic encoding of SHA-256(public key
      string), e.g. first 8 hex characters grouped as `7C91-42AE`;
    - PIN DOWN THE EXACT ENCODING ONCE, IN CODE, BEFORE INTEGRATION:
      byte count taken from the hash, hex vs base32, and grouping. Once
      agreed, put it in one shared function both client paths call — do
      not let two different parts of the codebase compute it differently;
    - this is an identity-verification aid, not a password/secret.

7.3 SENDER ENCRYPTION — `pandora send`
    1. take plaintext/file input;
    2. resolve recipient handle -> fetch their public key from the relay;
    3. compute and DISPLAY the recipient's fingerprint;
    4. HARD STOP: `Verify device 7C91-42AE? [y/N]` — do not proceed
       without explicit confirmation. This prompt is not optional UX, it
       is the mechanism that defeats relay key-substitution attacks. Do
       not let anyone strip it out for demo speed;
    5. on confirmation: `age.Encrypt(buf, recipientPublicKey)`, write
       plaintext into the returned io.Writer, close it to finalize;
    6. upload the resulting ciphertext bytes + ttl/burn flag to the relay;
    7. receive and display the share ID/link.

7.4 RECIPIENT DECRYPTION — `pandora read <id-or-link>`
    1. fetch the ciphertext from the relay (see section 9 for GET vs
       consume semantics);
    2. `age.Decrypt(ciphertextReader, localIdentity)`;
    3. on failure (wrong key OR tampered ciphertext — age does not
       distinguish these to the caller, and neither should the CLI's
       error message): print a clear, generic failure, e.g. "This secret
       could not be decrypted. It may not be addressed to this device, or
       it may have been tampered with." Never show partial/garbage output;
    4. on success: print plaintext to stdout. Do not write it to a
       persistent file unless the user passes an explicit --save flag.


================================================================================
7. MVP — WHAT MUST ACTUALLY WORK (36-hour scope)
================================================================================
MVP-1  DEVICE INIT — `pandora init` generates and persists local identity.
MVP-2  DEVICE IDENTITY — `pandora identity` shows handle + fingerprint.
MVP-3  LOCAL ENCRYPTION — sender encrypts entirely locally, via `age`.
MVP-4  DEVICE-BOUND ENCRYPTION — encryption targets a specific recipient
       public key, fetched from the relay by handle.
MVP-5  LOCAL DECRYPTION — the authorized recipient decrypts successfully.
MVP-6  UNAUTHORIZED DEVICE FAILURE — a different device's identity fails
       to decrypt. THIS IS THE MOST IMPORTANT DEMO TEST — do not treat it
       as "obviously it'll work," explicitly test it with a THIRD identity.
MVP-7  MANDATORY FINGERPRINT VERIFICATION — hard-stop confirmation before
       every encryption, no bypass flag.
MVP-8  BACKEND RELAY — upload/retrieve ciphertext via share ID/URL.
MVP-9  TTL — expired payloads become unavailable.
MVP-10 BURN AFTER READING — atomic (Redis GETDEL), race-safe.
MVP-11 PLAIN CLI — every one of the above reachable via plain flag-based
       commands, with no dependency on Bubble Tea being finished.

EXPLICITLY NOT MVP FOR THIS BUILD (cut, not "later if time"):
    browser/WASM evaluation sandbox; Ed25519 device-identity/signing
    lifecycle; multi-device roster; device revocation; mobile app; browser
    extension; user accounts beyond the handle/public-key registration;
    file previews; complex permissions; multi-recipient send (the `age`
    library supports multiple recipients natively if there's real slack
    at the very end, but don't design around it).

STRETCH, ONLY AFTER MVP-1 THROUGH MVP-11 ARE DONE AND REHEARSED:
    Bubble Tea interactive TUI layered on top of the plain CLI; basic
    rate limiting on the relay; cloud deployment (see section 8).


================================================================================
8. WHAT TO CUT OR RECONSIDER GIVEN 36 HOURS
================================================================================
CUT: browser WASM/xterm.js demo sandbox. Live demo = two real terminals.
DEMOTED: Bubble Tea TUI is garnish layered on a working plain CLI, never
   the thing the demo depends on.
RECONSIDER: cloud deployment (Render/Fly.io/Railway). If CloneFest doesn't
   require a public URL for asynchronous judging, running everything
   locally via `docker-compose up` for a live/in-person demo removes an
   entire failure category (DNS, cold starts, env mismatches) for little
   grading upside. Check the actual submission requirements before
   committing Pavan's later hours to this.
CUT (from any earlier testing plan): Unicode edge cases, large-payload
   testing, adversarial fuzzing beyond basic input validation. Testing
   this build covers exactly three properties — see section 15.
GUT-CHECK NOW, BEFORE PHASE 1: if nobody on the team is comfortable
   writing Go already, that is the single biggest risk in this entire
   plan — bigger than any architecture decision above. Say so out loud
   immediately if it's true; don't discover it at hour 20.


================================================================================
9. BACKEND RELAY — API CONTRACT
================================================================================
The backend is intentionally dumb: register public keys/handles, store
and serve encrypted payloads, enforce expiration, delete burn-after-
reading payloads atomically. It never decrypts, never inspects plaintext,
never holds a private key.

POST /keys
    Request:  { handle: string (optional, server generates if omitted),
                public_key: string }
    Response: { handle, fingerprint }
    Reject (409) if handle already registered — never silently overwrite
    a registered key, that's exactly the substitution the fingerprint
    check exists to catch.

GET /keys/:handle
    Response: { public_key, fingerprint } or 404. Fully public data, no
    auth required.

POST /paste
    Request:  { ciphertext (age-encrypted bytes), ttl_seconds,
                burn_after_reading: bool }
    Response: { id }
    Never logged: ciphertext body.

GET /paste/:id
    IMPORTANT — branch on burn_after_reading:
      if burn_after_reading: use Redis GETDEL (atomic fetch+delete) so two
      simultaneous reads cannot both succeed;
      if not burn_after_reading: use a plain GET honoring Redis TTL for
      expiry, do NOT delete on read.
    Missing this branch means every paste silently becomes burn-after-
    reading on first read — verify this explicitly in testing.
    Response: { ciphertext } or 404 (expired/not found/already consumed —
    a single generic "not found" response for all three is fine and
    avoids leaking which case occurred).

GET /health — trivial liveness check for deployment sanity.


================================================================================
10. GITHUB REPOSITORY STRUCTURE
================================================================================
pandoras-veil/
|
+-- cmd/
|   +-- pandora/
|       +-- main.go                 -- CLI entrypoint (plain commands first)
|
+-- internal/
|   +-- crypto/
|   |   +-- identity.go             -- age identity generation/storage
|   |   +-- fingerprint.go          -- shared fingerprint function (7.2)
|   |   +-- encrypt.go              -- thin wrapper around age.Encrypt
|   |   +-- decrypt.go              -- thin wrapper around age.Decrypt
|   |
|   +-- storage/
|   |   +-- local_identity.go       -- read/write ~/.pandora/identity (0600)
|   |
|   +-- client/
|   |   +-- api.go                  -- HTTP calls to the relay
|   |
|   +-- tui/                        -- STRETCH ONLY, built after plain CLI
|       +-- app.go
|       +-- send.go
|       +-- read.go
|       +-- identity.go
|
+-- server/
|   +-- main.go
|   +-- handlers/
|   |   +-- keys.go                 -- POST /keys, GET /keys/:handle
|   |   +-- paste.go                -- POST /paste, GET /paste/:id
|   +-- storage/
|       +-- redis.go                -- GETDEL / TTL logic (section 9)
|
+-- docs/
|   +-- THIS FILE (or a copy) as README.md
|   +-- THREAT_MODEL.md              -- can be extracted from section 5
|
+-- tests/
|   +-- crypto/                      -- the 3 properties, section 15
|   +-- integration/                 -- end-to-end send/read across 2 keys
|
+-- go.mod
+-- go.sum
+-- README.md
+-- LICENSE
+-- .gitignore

Keep crypto, API, storage, and CLI/TUI responsibilities in separate
packages — no giant monolithic main.go.

BRANCH MAPPING (matches the 5 branches already created in the repo):
    main                    -- stable submission branch only
    dev                     -- integration branch, everything merges here
                               first, then dev -> main after end-to-end
                               validation
    feature-crypto-core     -- Pranav: internal/crypto/, internal/storage/
    feature-backend-relay   -- Pavan: server/
    feature-tui-sandbox     -- Ujwal: cmd/pandora (plain CLI first), then
                               internal/tui/ as a stretch layer
Nobody commits directly to main. Feature branches merge to dev; dev merges
to main only after the full end-to-end test in section 12 passes.


================================================================================
11. TEAM DIVISION OF RESPONSIBILITIES
================================================================================
PRANAV — Team Leader, Cryptographic Core, Integration Owner
    Branch: feature-crypto-core
    Owns: internal/crypto/, local identity storage, encrypt/decrypt via
    `age`, fingerprint generation, final client<->backend integration.
    Can work independently on: key generation, fingerprint function,
    encrypt/decrypt wrappers, unit tests for all of the above — UNTIL the
    payload shape and API contract need to be agreed with Pavan (section
    12, coordination point 1, immediately).

PAVAN — Backend Relay Lead
    Branch: feature-backend-relay
    Owns: server/, Redis connection, all four API routes, TTL, GETDEL,
    deployment (if pursued — see section 8), health endpoint.
    Can work independently on: the entire server, using a fake/mock
    ciphertext blob for testing — UNTIL the real payload format from
    Pranav needs to be stored (coordination point 1).

UJWAL — CLI/TUI + Demo Lead
    Branch: feature-tui-sandbox
    Owns: cmd/pandora (the plain CLI, build this first), later
    internal/tui/ as a stretch layer, the fingerprint confirmation
    prompt's exact wording/UX, the live two-terminal demo choreography.
    Can work independently on: CLI argument parsing, output formatting,
    prompt flows against mocked crypto/API calls — UNTIL the CLI needs to
    call Pranav's real crypto functions and Pavan's real API client
    (coordination point 2).


================================================================================
12. COORDINATION POINTS (mandatory syncs — do not skip these)
================================================================================
COORDINATION POINT 1 — IMMEDIATELY (first 60 minutes)
    Pranav + Pavan + Ujwal, all three.
    Freeze together: the four API routes (section 9), the ciphertext
    payload shape (just bytes + ttl + burn flag, per section 6's
    simplification), the handle/fingerprint format, error response shape.
    Do not start writing code that touches any of these before this sync
    happens. This is the single most important early synchronization
    point — get it exactly right once instead of renegotiating it later.

COORDINATION POINT 2 — AFTER LOCAL CRYPTO WORKS (roughly hour 5)
    Pranav + Ujwal.
    Freeze: CLI command names/flags, input/output expectations, the exact
    fingerprint confirmation prompt text, success/failure message wording.

*** HARD CHECKPOINT — HOUR 12 ***
    ALL THREE.
    Goal: Device A encrypts -> relay stores -> Device B decrypts,
    end-to-end, for real, not mocked.
    If this does NOT work by hour 12: everyone stops adding anything new
    and fixes integration. Cut order if still behind after that: (1) any
    deployment/cloud work, (2) Bubble Tea TUI work, (3) anything not in
    the MVP list in section 7. Do not touch testing time (section 15) to
    buy back a cut feature — testing is what makes MVP-6 (unauthorized
    device fails) actually trustworthy in front of a judge.

ROUGH DEMO DRY-RUN — HOUR 13-14 (new, not in earlier drafts — add this)
    All three, informally, immediately after the hour-12 checkpoint
    passes. Run the full demo story (section 16) once, badly, unpolished.
    The goal is only to catch a broken narrative beat — e.g. the
    "unauthorized device fails" case not actually working, or a gap in
    the leak-the-link story — while there are still ~20 hours left to fix
    it, instead of discovering it during the real rehearsal near the end.

COORDINATION POINT 3 — FINAL INTEGRATION (roughly hour 27-30)
    ALL THREE. Verify the full chain: CLI -> Go crypto -> backend ->
    Redis -> (deployed service, if pursued). Then freeze all features —
    no new code after this except fixes found in rehearsal.


================================================================================
13. 36-HOUR EXECUTION PLAN
================================================================================
HOUR 0-1   — Coordination Point 1 (section 12). Repo/branch setup already
             done. Agree environment variables, local dev commands.

HOUR 1-5   — PRANAV: init, key generation, fingerprint function, local
             encrypt/decrypt via `age`.
             PAVAN: Go server skeleton, Redis connection, all 4 routes
             against a mock payload.
             UJWAL: plain CLI skeleton (cmd/pandora) — init, identity,
             send, read as flag-based commands, calling stubs.

HOUR 5-8   — PRANAV: payload serialization finalized against the real
             API contract; unit tests for crypto (section 15).
             PAVAN: Redis TTL + GETDEL finished and branch-tested
             (burn vs non-burn path, per section 9).
             UJWAL: fingerprint confirmation prompt wired to real
             (not mocked) crypto/client calls — Coordination Point 2.

HOUR 8-12  — ALL THREE: first real end-to-end integration. This is the
             hard checkpoint. Do not polish anything else until this
             passes.

HOUR 13-14 — Rough demo dry-run (section 12). Fix whatever broke.

HOUR 14-16 — Security demonstration hardening: fingerprint verification
             enforced with no bypass; unauthorized-device rejection
             explicitly tested with a third identity; tampered-ciphertext
             rejection tested; TTL and burn-after-reading verified.

HOUR 16-22 — Bubble Tea TUI polish (Ujwal), IF the plain CLI is fully
             solid — otherwise this time goes to CLI robustness instead.
             Pranav integrates any remaining real crypto/client calls.
             Pavan improves backend error messages/reliability.

HOUR 22-27 — Deployment, ONLY if pursued per section 8's reconsideration.
             Otherwise: this time goes to testing (section 15) and demo
             choreography instead.

HOUR 27-31 — Full demo rehearsal, the real one. Run the complete story in
             section 16 start to finish, more than once.

HOUR 31-34 — Fix only: crashes, broken demo steps, confusing UX,
             integration bugs. NO new features, full stop.

HOUR 34-36 — Freeze: merge feature branches -> dev -> main. Final README
             check, no secrets committed, environment configuration
             verified, repo/deployment sanity check.


================================================================================
14. DEFINITION OF DONE
================================================================================
[ ] Device can initialize and shows a stable identity + fingerprint.
[ ] Sender selects a recipient and sees their fingerprint before encrypting.
[ ] Encryption is impossible without explicit fingerprint confirmation.
[ ] Secret is encrypted entirely locally, via `age`.
[ ] Relay never receives plaintext.
[ ] Authorized recipient decrypts successfully.
[ ] A third, unauthorized device fails to decrypt the same ciphertext.
[ ] Tampered ciphertext is rejected cleanly, no partial output.
[ ] Share link/ID round-trips correctly.
[ ] TTL expiry works.
[ ] Burn-after-reading is atomic (two simultaneous reads, one succeeds).
[ ] Every one of the above works via the plain CLI, independent of TUI.
[ ] Demo has been rehearsed start to finish at least twice.


================================================================================
15. TESTING SCOPE (deliberately compressed for 36 hours)
================================================================================
Exactly three properties, nothing more:
1. Authorized device -> DECRYPTS.
2. Unauthorized device -> CANNOT decrypt (test with a real third identity,
   not an assumption).
3. Tampered ciphertext -> fails cleanly, no partial/garbage plaintext.
Everything from earlier, longer-timeline testing plans (Unicode edge
cases, large payloads, adversarial fuzzing, exhaustive API validation) is
explicitly out of scope for this build. Say this out loud to the team so
nobody quietly spends an hour on it out of habit from an earlier draft.


================================================================================
16. FINAL DEMONSTRATION STORY
================================================================================
The judge should understand this in under 60 seconds:

"Pandora's Veil is a device-bound secret-sharing system. Unlike a
traditional secret link, the URL alone does not authorize access.

The sender's machine encrypts locally, using a native Go client — never a
browser. Before encrypting, the sender verifies the recipient's
cryptographic device fingerprint out of band. The server only ever stores
encrypted material.

Now I'll leak the link intentionally.

[On an unauthorized device] — ACCESS DENIED.
[On the authorized device] — DECRYPTED SUCCESSFULLY.

The link locates the secret. The device authorizes the decryption. Even
if the link leaks, the secret doesn't."

Demo sequence: 1) initialize Device A and Device B, show both
fingerprints; 2) send a secret A -> B, showing the mandatory fingerprint
verification step; 3) generate the link; 4) read successfully on B;
5) attempt the same link on a third device C, show rejection; 6) show the
backend only ever stored ciphertext (query it live); 7) demonstrate TTL
or burn-after-reading.


================================================================================
17. RULES FOR ANY LLM WORKING ON THIS PROJECT
================================================================================
1. Understand this document before writing code.
2. Do not replace `age`-based encryption with custom cryptography.
3. Do not move production encryption/decryption into browser JavaScript.
4. Do not upload private keys to the backend, ever.
5. Do not remove the mandatory fingerprint verification prompt.
6. Do not make the URL itself sufficient for decryption.
7. Do not silently change the payload structure once integration has
   started — that requires Coordination Point 1 to be reopened, briefly,
   with all three people present.
8. Do not add features that jeopardize the 36-hour MVP in section 7.
9. Prefer small, testable modules over one large implementation.
10. When changing crypto behavior: state exactly what assumption is
    changing and coordinate with Pranav before writing code.
11. When changing API payloads/routes: coordinate with Pavan first.
12. When changing CLI/TUI contracts: coordinate with Ujwal first.
13. Always test the two most important properties before anything else:
    authorized device decrypts; unauthorized device cannot.
14. Do not write another full revision of this document. Small, targeted
    edits only, and only to correct something now factually wrong.


================================================================================
18. FINAL PROJECT IDENTITY
================================================================================
PROJECT:               Pandora's Veil
CATEGORY:               Secure Secret Sharing / Device-Bound Cryptographic
                        Relay / Privacy Tool
PRIMARY STACK:          Go, age (X25519), Redis, (Bubble Tea — stretch)
PRIMARY DIFFERENTIATOR: Device-bound decryption
SECONDARY DIFFERENTIATOR: Mandatory out-of-band fingerprint verification
                        (lead with this one — see section 3)
UX DIFFERENTIATOR:      Native CLI-first, no browser trust re-established
                        per visit
SECURITY DIFFERENTIATOR: Server stores encrypted payloads only, never
                        plaintext, never a private key
DEMO DIFFERENTIATOR:    Leak the link -> unauthorized device still fails
NORTH STAR:             "Even if the link leaks, the secret doesn't."

================================================================================
END OF PANDORA'S VEIL — FINAL MASTER README (36-HOUR BUILD)
================================================================================
