# 🔒 Pandora's Veil
### Device-Bound Zero-Knowledge Secret Relay & Real-Time Terminal Chat

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Cryptography](https://img.shields.io/badge/Crypto-filippo.io%2Fage-black?style=flat)](https://filippo.io/age)
[![Backend Relay](https://img.shields.io/badge/Cloud%20Relay-Render%20Live-46E3B7?style=flat)](https://pandoras-veil.onrender.com/health)
[![Storage](https://img.shields.io/badge/Storage-Redis%20GETDEL-red?style=flat&logo=redis)](https://redis.io/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **"The share link locates the secret. The authorized device authorizes decryption."**
> 
> *North Star Guarantee: "Even if the link leaks, the secret doesn't."*

---

## 📌 Overview

**Pandora's Veil** is a native, device-bound zero-knowledge secret-sharing and real-time messaging system. It eliminates the structural vulnerabilities of traditional in-browser pastebins (e.g. PrivateBin) by executing all cryptographic operations inside a native compiled binary where private keys never leave physical devices.

### The Problem We Solve
1. **Per-Visit Browser Trust**: In-browser JavaScript encryption forces users to re-trust the web server on every page load. A compromised web server or CDN can inject malicious JS to intercept secrets.
2. **Link-Is-The-Only-Guard**: Anyone who acquires a traditional paste link (via Slack/Discord leaks, shoulder surfing, or compromised clipboard) can decrypt the secret.
3. **No True Device Authorization**: Standard secret bins protect data in transit, but cannot guarantee that only a specific physical laptop or server can unlock the payload.

### Our Solution
- **Native Compiled Client (`pv`)**: Cryptography executes in a compiled Go binary. Trust is established once at installation, completely bypassing browser JavaScript vulnerabilities.
- **Device-Bound Encryption (`filippo.io/age`)**: Secrets are encrypted locally targeting the recipient's physical X25519 identity. Possession of the URL alone grants zero access—decryption strictly requires the recipient's local private key.
- **Mandatory Hard-Stop Fingerprint Verification**: Senders must explicitly verify the recipient's 8-character device fingerprint out-of-band before encryption occurs, completely defeating relay key-substitution (MITM) attacks.
- **Real-Time WhatsApp-Style Terminal Chat (`pv chat`)**: High-speed, end-to-end encrypted messaging stream directly in the command terminal with instant delivery, timestamps, and message bubble alignment.
- **Live Deployed Cloud Relay**: Backed by a high-availability cloud backend at `https://pandoras-veil.onrender.com`.

---

## 🚀 How to Install & Use Pandora's Veil

### 1. Installation

Clone the repository and install the `pv` command globally:

#### On Windows (PowerShell):
```powershell
git clone https://github.com/Mob-max30/pandoras-veil.git
cd pandoras-veil
go build -o "$env:GOPATH\bin\pv.exe" ./cmd/pandora
```

#### On Linux / macOS (Bash / Zsh):
```bash
git clone https://github.com/Mob-max30/pandoras-veil.git
cd pandoras-veil
go build -o /usr/local/bin/pv ./cmd/pandora
```

*Verify installation:*
```powershell
pv version
```

---

### 2. Initialize Your Device

Every user runs this one-time command to generate their local X25519 keypair and register their handle on the live cloud relay:

```powershell
pv init --handle YOUR_NAME
```

**Example Output:**
```text
[i] Generating new X25519 device keypair...
[i] Registering public key with relay (https://pandoras-veil.onrender.com)...
[✓] Device initialized successfully!

Device Credentials:
  Handle:      ALICE
  Fingerprint: 47B7-9F60
  Public Key:  age1mfl9w8z7q2y0nuajrct03vxfdhngd5dyszv2j0vt02ssdmq873tzq7ddx8
  Config File: ~/.pandora/identity.json (0600 permissions)
```

---

### 3. Check Device Identity

Inspect your device credentials and verify them against the live cloud relay:

```powershell
pv identity
```

**Output:**
```text
Device Identity (Verified on Relay):
  Handle:      ALICE
  Fingerprint: 47B7-9F60
  Public Key:  age1mfl9w8z7q2y0nuajrct03vxfdhngd5dyszv2j0vt02ssdmq873tzq7ddx8

Security Tip: Share your Handle or Fingerprint out-of-band with senders to verify authenticity.
```

---

### 4. Real-Time Encrypted Live Terminal Chat

Start an interactive, end-to-end encrypted chat with another user on the live cloud relay:

**Alice runs:**
```powershell
pv chat --with BOB
```

**Bob runs:**
```powershell
pv chat --with ALICE
```

**Live WhatsApp-Style Interface:**
```text
================================================================================
  🔒 PANDORA LIVE RELAY | End-to-End Encrypted Session with BOB
  Device Fingerprint: [915E-B66D] | Zero Knowledge Relay Active
  Type your message and press [Enter] to send live. Press [Ctrl+C] to exit.
================================================================================

[15:34:05] [BOB] ❯ Hello Alice! Is this session end-to-end encrypted?
                                  Yes, 100% encrypted! [YOU] [15:34:10]

[ALICE] > 
```
- **Incoming Messages**: Formatted on the left edge with sender handle.
- **Outgoing Messages**: Right-aligned on the terminal screen with pink `[YOU]` badge, green text, and gray timestamps.

---

### 5. Sending Device-Bound Secrets

#### Sending a Text Secret:
```powershell
pv send --to BOB "Confidential API Key: sk_live_998877665544"
```

The CLI requires mandatory out-of-band fingerprint confirmation before encrypting:
```text
================ RECIPIENT VERIFICATION ================
  Recipient Handle:      BOB
  Device Fingerprint:    915E-B66D
  Target Public Key:     age1t02ssdmq873tzq7ddx82q7ptk62q7y0nuajrct03vxfdhngd5dyszv2j0v
========================================================
[SECURITY CHECK] Confirm that the fingerprint matches the recipient's device out-of-band.

Verify device 915E-B66D? [y/N]: y

[i] Encrypting secret locally for recipient device key...
[i] Uploading encrypted envelope to relay (https://pandoras-veil.onrender.com)...
[✓] Secret encrypted and deposited successfully!

Share Details:
  Share ID:    pbqvqkuyrnaprbwqgyuin7nwy
  Share Link:  https://pandoras-veil.onrender.com/paste/pbqvqkuyrnaprbwqgyuin7nwy
  Target:      BOB (Fingerprint: 915E-B66D)
  Policy:      TTL 86400 seconds
```

#### Sending a File:
```powershell
pv send --to BOB --file ./confidential_report.pdf
```

#### Self-Destruct / Burn-After-Reading:
Add `--burn` to ensure the secret is atomically deleted from the server immediately after the first read:
```powershell
pv send --to BOB --burn "One-Time Recovery Code: 839201"
```

---

### 6. Decrypting Secrets on Authorized Device

On Bob's device:

```powershell
pv read pbqvqkuyrnaprbwqgyuin7nwy
```

**Output on Bob's Device (Authorized):**
```text
[i] Fetching encrypted secret 'pbqvqkuyrnaprbwqgyuin7nwy' from relay...
[i] Attempting device-bound decryption...
[✓] Decryption successful! (Authorized device key matched)

================ DECRYPTED SECRET ================
Confidential API Key: sk_live_998877665544
==================================================
```

#### Saving Plaintext to a File:
```powershell
pv read pbqvqkuyrnaprbwqgyuin7nwy --save ./decrypted_key.txt
```

#### Unauthorized Attacker (Eve):
If an attacker intercepts the link and runs `pv read`:
```powershell
pv read pbqvqkuyrnaprbwqgyuin7nwy
```
**Output:**
```text
[i] Fetching encrypted secret 'pbqvqkuyrnaprbwqgyuin7nwy' from relay...
[i] Attempting device-bound decryption...
[✗] ACCESS DENIED: this secret could not be decrypted (it may not be addressed to this device, or it may have been tampered with)
```

---

## 🛠️ Complete CLI Command Reference

| Command | Usage | Description |
| :--- | :--- | :--- |
| `pv init` | `pv init [--handle <name>] [--force]` | Generates local X25519 identity and registers public key on relay. |
| `pv identity` | `pv identity` | Displays device handle, public key, and fingerprint verified against live relay. |
| `pv send` | `pv send --to <handle> [options] [text]` | Encrypts payload locally for target device and deposits ciphertext on relay. |
| `pv read` | `pv read <share-id-or-url> [--save <file>]` | Fetches ciphertext and decrypts using this device's private key. |
| `pv chat` | `pv chat --with <handle>` | Starts real-time, end-to-end encrypted terminal chat session. |
| `pv version`| `pv version` | Prints CLI version information. |
| `pv help` | `pv help` | Displays help and command options. |

### Available Flags:
- `--to <handle>`: Target recipient handle (e.g. `BOB`) or raw public key (`age1...`).
- `--file <path>`: Read secret payload from a file instead of arguments/stdin.
- `--save <path>`: Write decrypted plaintext directly to file (with `0600` permissions).
- `--burn`: Enable Burn-After-Reading (atomically destroys ciphertext after 1st read).
- `--ttl <seconds>`: Set expiration lifespan (default: `86400` = 24 hours).
- `--relay <url>`: Custom relay URL (default: `https://pandoras-veil.onrender.com`).
- `--config <path>`: Custom local identity configuration file path.

---

## 🛡️ Security Architecture

```
+------------------+                      +-----------------------+                      +------------------+
|   Alice Device   |                      |  Pandora Relay Server |                      |    Bob Device    |
| (Private Key A)  |                      | (Zero Knowledge Cloud)|                      | (Private Key B)  |
+------------------+                      +-----------------------+                      +------------------+
        │                                             │                                            │
        │─── 1. Query Bob's Public Key & FP ─────────►│                                            │
        │◄── 2. Returns age1bob... (FP: 915E-B66D) ───│                                            │
        │                                             │                                            │
 [Alice verifies FP]                                  │                                            │
 [Encrypts locally via age]                           │                                            │
        │                                             │                                            │
        │─── 3. Uploads Ciphertext (Base64) ─────────►│                                            │
        │                                       [Redis GETDEL]                                     │
        │                                             │                                            │
        │                                             │◄── 4. Bob fetches ciphertext with ID ──────│
        │                                             │─── 5. Returns Ciphertext Blob ────────────►│
        │                                             │                                            │
        │                                             │                                     [Bob decrypts]
        │                                             │                                     [with local key]
```

---

## 🧪 Testing & Verification

Run the entire test suite across client and server:

```powershell
# Run CLI & Client test suites
go test -v ./...

# Run Backend Relay test suite
cd server
go test -v ./...
cd ..
```

---

## 👥 Project Team

- **Ujwal** — CLI/TUI, Real-Time Terminal Chat, Interactive UX & Demo Lead
- **Pavan** — Backend Relay, Redis GETDEL/TTL, Cloud Deployment & API Architecture
- **Pranav** — Cryptographic Architecture, `age` Primitives & Integration

---

## 📄 License
This project is licensed under the [MIT License](LICENSE).
