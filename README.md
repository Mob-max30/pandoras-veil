# 🔒 Pandora's Veil
### Device-Bound Zero-Knowledge Secret Relay

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Cryptography](https://img.shields.io/badge/Crypto-filippo.io%2Fage-black?style=flat)](https://filippo.io/age)
[![Storage](https://img.shields.io/badge/Backend-Redis%20GETDEL-red?style=flat&logo=redis)](https://redis.io/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **"The share link locates the secret. The authorized device authorizes decryption."**
> 
> *North Star: "Even if the link leaks, the secret doesn't."*

---

## 📌 Overview

**Pandora's Veil** is a native, device-bound secret-sharing system designed to securely transmit sensitive text payloads without relying on browser-delivered code or fragile link-only security models.

### The Problem We Solve
Traditional pastebin and disposable secret sharing tools (e.g. PrivateBin) suffer from two structural vulnerabilities:
1. **Per-Visit Browser Trust**: In-browser JavaScript encryption requires re-establishing trust with the server on every page load. A compromised server can serve modified JS to steal plaintext.
2. **Link-Is-The-Only-Guard**: Anyone who acquires the share URL (via copy-paste leak, chat logs, or shoulder surfing) can decrypt the content.

### Our Solution
- **Native Go Client**: Cryptography executes in a compiled native binary. Trust is established once at installation, avoiding per-visit browser JS execution.
- **Device-Bound Encryption**: Secrets are encrypted specifically for the recipient's public key (`filippo.io/age`). Possession of the URL alone grants zero access—decryption requires the recipient's local private key.
- **Mandatory Fingerprint Check**: Senders must explicitly verify the recipient's device fingerprint out-of-band before encryption, defeating relay key-substitution (MITM) attacks.

---

## 🛡️ Security Philosophy & Threat Model

### Core Principles
1. **Local Encryption**: Plaintext enters the native client; the server never receives plaintext.
2. **Device Isolation**: Private keys are generated locally (`~/.pandora/identity` with `0600` permissions) and are **never uploaded or transmitted**.
3. **Established Primitives**: Powered strictly by `filippo.io/age` (X25519, HKDF, ChaCha20-Poly1305). No custom cryptography.
4. **Out-of-Band Verification**: Senders must confirm recipient fingerprints before encrypting.
5. **Zero-Knowledge Payload Storage**: The server stores only ciphertext bytes and lifecycle metadata (TTL, burn flag).

### Threat Matrix

| Mitigated Risks | Non-Mitigated Risks (Out of Scope) |
| :--- | :--- |
| ✅ Compromised browser JS delivery | ❌ Compromised sender/recipient OS |
| ✅ Malicious relay key substitution (MITM) | ❌ Stolen recipient private key |
| ✅ Accidental link leakage | ❌ Adversarial physical device access |
| ✅ Database / Redis storage exposure | ❌ Retroactive decryption if static private key leaks |

> ℹ️ **Honest Metadata Disclosure**: The relay can observe request timestamps, ciphertext sizes, IP/routing metadata, TTLs, and handle lookups. We claim zero-knowledge of *payload contents*, not absolute metadata anonymity.

---

## 🔑 Cryptographic Architecture

We utilize `filippo.io/age` as the cryptographic boundary:

```
[ Sender CLI ] ──( 1. Fetch Public Key & Verify Fingerprint )──> [ Relay Server ]
       │                                                                │
  2. Encrypt locally                                                    │
     via age.Encrypt()                                                  │
       │                                                                │
       └───( 3. Upload Ciphertext Bytes + TTL / Burn Flag )────────────►│
                                                                        │
                                                                   [ Redis Store ]
                                                                        │
[ Recipient CLI ] ◄──( 4. Download Ciphertext & Decrypt locally )───────┘
                       via age.Decrypt(localIdentity)
```

1. **Identity Generation (`pandora init`)**: Generates X25519 identity using `age.GenerateX25519Identity()`. Saved locally to `~/.pandora/identity` with restricted `0600` permissions.
2. **Device Fingerprint**: Deterministic encoding of `SHA-256(public_key)` (formatted as `7C91-42AE`).
3. **Mandatory Verification**: `pandora send` prompts: `Verify device 7C91-42AE? [y/N]`. Hard-stop check before calling `age.Encrypt`.
4. **Atomic Burn-After-Reading**: The relay uses Redis `GETDEL` for burn-on-read pastes, ensuring race-safe single-consumption.

---

## 📡 Relay API Contract

The backend relay is intentionally minimal: handles key registration, ciphertext persistence, TTL, and atomic deletion.

| Endpoint | Method | Request Payload | Response / Action |
| :--- | :--- | :--- | :--- |
| `/keys` | `POST` | `{ "handle": "PV-1234", "public_key": "age1..." }` | Register handle. Rejects (409) on handle collision. |
| `/keys/:handle` | `GET` | N/A | Returns `{ "public_key": "...", "fingerprint": "..." }` |
| `/paste` | `POST` | `{ "ciphertext": "<bytes>", "ttl_seconds": 3600, "burn_after_reading": true }` | Returns `{ "id": "..." }` |
| `/paste/:id` | `GET` | N/A | Evaluates `burn_after_reading`: uses Redis `GETDEL` if burn, `GET` if TTL only. Returns `{ "ciphertext": "..." }`. |
| `/health` | `GET` | N/A | Liveness status check (`200 OK`). |

---

## 📂 Repository Structure

```
pandoras-veil/
├── cmd/
│   └── pandora/
│       └── main.go                 # CLI entrypoint
├── internal/
│   ├── crypto/
│   │   ├── identity.go             # age identity generation & storage
│   │   ├── fingerprint.go          # SHA-256 fingerprint encoder
│   │   ├── encrypt.go              # age.Encrypt wrapper
│   │   └── decrypt.go              # age.Decrypt wrapper
│   ├── storage/
│   │   └── local_identity.go       # ~/.pandora/identity read/write
│   ├── client/
│   │   └── api.go                  # Relay HTTP client
│   └── tui/
│       ├── app.go                  # Bubble Tea TUI app (stretch)
│       ├── send.go
│       ├── read.go
│       └── identity.go
├── server/
│   ├── main.go                     # Relay server entrypoint
│   ├── handlers/
│   │   ├── keys.go                 # Key registration handlers
│   │   └── paste.go                # Ciphertext store/fetch handlers
│   └── storage/
│       └── redis.go                # Redis GETDEL & TTL operations
├── docs/
│   └── THREAT_MODEL.md             # Threat model breakdown
├── tests/
│   ├── crypto/                     # Core cryptographic property tests
│   └── integration/                # End-to-end integration tests
├── go.mod
├── go.sum
├── LICENSE
└── .gitignore
```

---

## 👥 Team & Responsibilities

| Team Member | Role | Primary Ownership | Git Branch |
| :--- | :--- | :--- | :--- |
| **Pranav** | Team Leader & Crypto Lead | Core Crypto (`internal/crypto`), Storage, Integration | `feature-crypto-core` |
| **Pavan** | Backend Relay Lead | Relay Server (`server/`), Redis GETDEL/TTL, API Routes | `feature-backend-relay` |
| **Ujwal** | CLI/TUI & Demo Lead | CLI Entrypoint (`cmd/pandora`), Prompt UX, Bubble Tea TUI | `feature-tui-sandbox` |

### Branching Strategy
- `main`: Production submission branch.
- `develop`: Integration branch (feature branches merge to `develop` first).
- Feature branches (`feature-crypto-core`, `feature-backend-relay`, `feature-tui-sandbox`): Individual work streams.

---

## ⏱️ 36-Hour Execution Timeline

- **Hours 0–1**: Coordination Sync 1 (API Contracts & Payload Formats Frozen)
- **Hours 1–5**: Core Implementations (`pandora init`, Server Skeleton, Plain CLI Skeleton)
- **Hours 5–8**: Crypto Wrappers, Redis TTL/GETDEL, Fingerprint Verification Prompt
- **Hour 12 [HARD CHECKPOINT]**: End-to-End Test (Device A Encrypts ➔ Relay ➔ Device B Decrypts)
- **Hours 13–14**: Informal Demo Dry-Run
- **Hours 14–16**: Security Demonstration Hardening (Unauthorized Device Rejection Tests)
- **Hours 16–22**: Bubble Tea TUI Polish (Stretch)
- **Hours 27–31**: Final Live Demo Rehearsal
- **Hours 34–36**: Code Freeze, Main Merge & Final Verification

---

## 🧪 Testing Scope

Testing is strictly focused on three core security properties:
1. **Authorized Device**: Successfully decrypts the payload.
2. **Unauthorized Device**: Fails to decrypt (tested with a distinct 3rd identity).
3. **Tampered Payload**: Fails cleanly without partial or garbage output.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
