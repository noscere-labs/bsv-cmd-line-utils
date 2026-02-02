---
name: bsv-tx-tools
description: Build, broadcast, inspect, and dissect BSV transactions using Go CLI tools (carve, broadcast, prettytx, getraw, pick, txstatus, keygen, wifinfo). Use when creating transactions, sending satoshis, parsing raw tx hex, fetching transactions from WhatsOnChain, extracting tx fields for pipelines, checking broadcast status via ARC, generating key pairs, or inspecting WIF keys. Supports mainnet and testnet.
---

# BSV Transaction Tools

Eight Go CLI tools for BSV transaction lifecycle: key generation → tx building → broadcasting → inspection → status tracking.

All tools support stdin piping for Unix-style composition. Install from `~/noscere/repos/bsv-cmd-line-utils`:

```bash
cd ~/noscere/repos/bsv-cmd-line-utils && go install ./cmd/...
```

## Tools

### keygen — Generate BSV key pairs

```bash
keygen                        # Single mainnet key pair
keygen -t                     # Testnet key pair
keygen -c 5 -j                # 5 keys, JSON output
keygen -u                     # Uncompressed public key
```

Flags: `-t` testnet, `-c N` count (1-100), `-j` JSON, `-u` uncompressed.

### wifinfo — Inspect a WIF private key

```bash
wifinfo <wif>                 # Show pubkeys, addresses, network
wifinfo -j <wif>              # JSON output
echo <wif> | wifinfo          # From stdin
```

Detects network (mainnet/testnet) and compression automatically. Shows compressed + uncompressed pubkeys, addresses, and WIFs for both networks.

Flags: `-w` WIF via flag, `-j` JSON, `--no-color` plain output.

### carve — Build and sign transactions

```bash
carve -w <WIF> -a <address> -s 1000        # Send 1000 sats
carve -w <WIF> -a <address>                 # Send ALL funds (minus fees)
carve -w <WIF> -a <address> -s 1000 -t      # Testnet
carve -w <WIF> -a <address> -s 1000000 -n 10  # Split into 10 equal outputs
carve -w <WIF> -a <address> --debug         # Verbose UTXO selection
```

Outputs raw tx hex to stdout. Fetches UTXOs from WhatsOnChain, uses largest-first selection, auto-calculates fees (min 100 sats).

Flags: `-w` WIF (required), `-a` address (required), `-s` satoshis (0=send all), `-t` testnet, `-f` fee/KB (default 100), `-d` dust limit (default 1), `-n` split count, `--debug`.

### broadcast — Broadcast raw transactions via ARC

```bash
echo <rawtx> | broadcast              # Broadcast to mainnet
echo <rawtx> | broadcast -t           # Testnet
echo <rawtx> | broadcast -m           # Monitor until final state
echo <rawtx> | broadcast -m -p 10     # Monitor, poll every 10s
broadcast -r <rawtx>                  # From flag
```

Requires `config.yaml` with ARC endpoints (in executable dir or cwd):

```yaml
arc-mainnet:
  url: "https://api.taal.com"
  api_key: "your_key"
  timeout: "30s"
arc-testnet:
  url: "https://arc-test.taal.com"
  api_key: "your_key"
  timeout: "30s"
polling:
  interval: "3s"
  max_retries: 10
  backoff_factor: 1.5
```

Flags: `-r` raw hex, `-m` monitor, `-p` poll interval (seconds), `-t` testnet.

Status flow: `RECEIVED → STORED → ANNOUNCED_TO_NETWORK → SEEN_ON_NETWORK → MINED`

### txstatus — Check transaction status via ARC

```bash
txstatus <txid>                # Check by argument
txstatus <txid> -t             # Testnet
txstatus <txid> -m             # Monitor until final
echo <txid> | txstatus         # From stdin
```

Same `config.yaml` as broadcast. Flags: `-i` txid via flag, `-m` monitor, `-p` poll rate, `-t` testnet.

### getraw — Fetch raw transaction hex from WhatsOnChain

```bash
getraw <txid>                  # Mainnet
getraw <txid> -t               # Testnet
echo <txid> | getraw           # From stdin
getraw <txid> | prettytx       # Chain with parser
```

Flags: `-i` txid via flag, `-t` testnet.

### prettytx — Parse and display raw transactions

```bash
echo <rawtx> | prettytx                # Colorized breakdown
prettytx -r <rawtx>                    # From flag
prettytx --no-color -r <rawtx>         # Plain (for scripting)
getraw <txid> | prettytx               # Chain with fetcher
carve -w <WIF> -a <addr> -s 1000 | prettytx  # Preview before broadcast
```

Shows: version, inputs (prevtx, vout, script, sequence), outputs (value in sats+BSV, locking script), locktime, txid. Extracts P2PKH addresses from scripts.

Flags: `-r` raw hex, `--no-color`.

### pick — Extract specific fields from raw transactions

```bash
pick <rawtx> --txid                          # Transaction ID
pick <rawtx> --output 0                      # First output (serialized)
pick <rawtx> --output-script 0               # First output's locking script
pick <rawtx> --output-value 0                # First output's value (8-byte LE hex)
pick <rawtx> --input 0 --input 1             # First two inputs
pick <rawtx> --input-prevtxid 0              # First input's source txid
pick <rawtx> --input-script 0                # First input's unlocking script
pick <rawtx> --version --locktime            # Tx-level fields
echo <rawtx> | pick --txid                   # From stdin
getraw <txid> | pick --output-script 0       # Chain with getraw
```

All selectors repeatable. Outputs one hex string per line. Supports `file://path` and URL input.

Flags: `-o` output, `--output-script`, `--output-value`, `-i` input, `--input-script`, `--input-prevtxid`, `--input-prevout`, `--input-sequence`, `-v` version, `-l` locktime, `--txid`.

## Common Workflows

### Create, preview, and broadcast

```bash
carve -w $WIF -a $DEST -s 5000 | tee >(prettytx) | broadcast -m
```

### Fetch, inspect, and extract

```bash
getraw <txid> | prettytx                         # Human-readable view
getraw <txid> | pick --output-script 0           # Extract locking script
getraw <txid> | pick --txid --output-value 0     # Get txid + first output value
```

### Testnet workflow

```bash
keygen -t -j                                     # Generate testnet keys
carve -w $TESTNET_WIF -a $TESTNET_ADDR -s 1000 -t | broadcast -t -m
```

### Key inspection

```bash
wifinfo $WIF -j | jq '.mainnet.address.compressed'   # Extract mainnet address
```

## Fee Estimation

Tx size: ~10 bytes base + 148 bytes/input + 34 bytes/output.
Fee: `max(100, size × feePerKB / 1000)`. Default 100 sat/KB. Minimum floor: 100 sats.

## Notes

- `broadcast` and `txstatus` need `config.yaml` with ARC API keys
- `carve` and `getraw` use WhatsOnChain API directly (no auth, ~3 req/sec rate limit)
- All tools accept input from stdin, flags, or positional args
- WIF keys: mainnet prefix `5`/`K`/`L`, testnet prefix `c`/`9`
- Never commit WIF keys to version control
