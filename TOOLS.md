# BSV Transaction Tools - User Guide

A comprehensive suite of command-line tools for Bitcoin SV (BSV) transaction handling, building, broadcasting, and analysis.

## Table of Contents

- [Installation](#installation)
- [Tools Overview](#tools-overview)
  - [broadcast](#broadcast---transaction-broadcaster)
  - [carve](#carve---transaction-builder)
  - [prettytx](#prettytx---transaction-parser)
  - [getraw](#getraw---transaction-fetcher)
- [Configuration](#configuration)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Installation

Build and install all tools:

```bash
# Install broadcast
cd cmd/broadcast && go install && cd ../..

# Install carve
cd cmd/carve && go install && cd ../..

# Install prettytx
cd cmd/prettytx && go install && cd ../..

# Install getraw
cd cmd/getraw && go install && cd ../..
```

After installation, all tools will be available globally in your terminal.

---

## Tools Overview

### broadcast - Transaction Broadcaster

Broadcasts raw Bitcoin transactions to the BSV network using ARC (BSV Transaction Processing) endpoints.

#### Features
- Config-based mainnet/testnet endpoint management
- Transaction status monitoring with automatic polling
- Real-time transaction lifecycle tracking
- YAML configuration for ARC endpoints

#### Usage

```bash
# Basic broadcast from stdin
echo "010000..." | broadcast

# Broadcast using raw flag
broadcast -r "010000..."

# Broadcast to testnet
echo "010000..." | broadcast -t

# Broadcast and monitor until final state
echo "010000..." | broadcast -m

# Monitor with custom poll rate (default: 5 seconds)
echo "010000..." | broadcast -m -p 10
```

#### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--raw` | `-r` | Raw transaction hex to broadcast | - |
| `--monitor` | `-m` | Monitor transaction status until final state | false |
| `--poll-rate` | `-p` | Polling rate in seconds for monitoring | 5 |
| `--testnet` | `-t` | Use testnet configuration | false |

#### Transaction Status Flow

```
RECEIVED → STORED → ANNOUNCED_TO_NETWORK → SEEN_ON_NETWORK → MINED
```

Other possible states:
- `REJECTED` - Transaction rejected by network
- `DOUBLE_SPEND_ATTEMPTED` - Double spend detected

#### Configuration

The tool reads from `config.yaml` (see [Configuration](#configuration) section).

---

### carve - Transaction Builder

Creates and signs BSV transactions from a WIF private key, with smart UTXO selection and fee estimation.

#### Features
- Smart UTXO selection with largest-first algorithm
- Automatic fee estimation with 100 satoshi minimum floor
- Support for "send all" transactions
- Mainnet/testnet support
- Debug mode for verbose logging
- Automatic change output handling
- Dust limit protection

#### Usage

```bash
# Send specific amount
carve -w <WIF> -a <destination_address> -s 1000

# Send all funds (minus fees)
carve -w <WIF> -a <destination_address>

# Use testnet
carve -w <WIF> -a <testnet_address> -s 1000 -t

# Enable debug logging
carve -w <WIF> -a <address> -s 1000 --debug

# Custom fee rate (satoshis per kilobyte)
carve -w <WIF> -a <address> -s 1000 -f 200

# Custom dust limit
carve -w <WIF> -a <address> -s 1000 -d 50
```

#### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--wif` | `-w` | Source WIF private key (required) | - |
| `--address` | `-a` | Destination address (required) | - |
| `--sats` | `-s` | Amount in satoshis to send | 0 (send all) |
| `--testnet` | `-t` | Use testnet | false |
| `--fee-per-kb` | `-f` | Fee per kilobyte in satoshis | 100 |
| `--dust` | `-d` | Dust limit in satoshis | 1 |
| `--debug` | - | Enable debug logging | false |

#### How It Works

1. **Derive Address**: Extracts public key and derives P2PKH address from WIF
2. **Fetch UTXOs**: Queries WhatsOnChain API for all unspent outputs
3. **Select UTXOs**: Uses largest-first selection to minimize inputs while covering amount + fees
4. **Build Transaction**: Creates inputs, outputs (payment + change), and estimates fees
5. **Sign Transaction**: Signs all inputs with private key
6. **Output**: Prints raw transaction hex to stdout

#### Fee Calculation

The tool calculates fees based on estimated transaction size:
- Each input: ~148 bytes
- Each output: ~34 bytes
- Base overhead: ~10 bytes

**Minimum fee floor: 100 satoshis**

Formula: `fee = max(100, (tx_size * fee_per_kb) / 1000)`

#### UTXO Selection

The tool uses a **largest-first** algorithm:
1. Sort UTXOs by value (descending)
2. Add UTXOs until `total_value >= amount + estimated_fee`
3. Return minimal set that covers the transaction

This minimizes the number of inputs and reduces transaction size.

---

### prettytx - Transaction Parser

Parses and displays raw Bitcoin transactions in human-readable format with colorized output.

#### Features
- Colorized transaction breakdown
- Detailed input/output analysis
- Script hex display
- Satoshi to BSV conversion
- Locktime interpretation
- Optional color-free output for scripting

#### Usage

```bash
# Parse from stdin
echo "010000..." | prettytx

# Parse using raw flag
prettytx -r "010000..."

# Disable colors (for scripting)
prettytx -r "010000..." --no-color

# Chain with other tools
carve -w <WIF> -a <address> -s 1000 | prettytx
getraw <txid> | prettytx
```

#### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--raw` | `-r` | Raw transaction hex to parse | - |
| `--no-color` | - | Disable colored output | false |

#### Output Format

```
================================================================================
TRANSACTION BREAKDOWN
================================================================================

Version: 1 (0x00000001)

In-counter: 2

INPUTS:
--------------------------------------------------------------------------------

Input #0:
  Prev TX ID: abc123...
  Prev Vout: 0
  Script Length: 107 bytes
  Script (hex): 473044022...
  Sequence: 4294967295 (0xffffffff)

Out-counter: 2

OUTPUTS:
--------------------------------------------------------------------------------

Output #0:
  Value: 1000 satoshis (0.00001000 BSV)
  Script Length: 25 bytes
  Script (hex): 76a914...

nLockTime: 0 (0x00000000)
           (Not locked)

================================================================================
Transaction ID: def456...
================================================================================
```

#### Color Scheme

- **Headers**: Cyan + Bold
- **Labels**: Yellow
- **Section Headers**: Green/Blue (inputs/outputs)
- **Scripts**: Magenta
- **Values**: White/Green
- **Annotations**: Dim

---

### getraw - Transaction Fetcher

Fetches raw transaction data from the WhatsOnChain API.

#### Features
- Mainnet/testnet support
- Accepts txid via argument or stdin
- Integration with WhatsOnChain API

#### Usage

```bash
# Fetch by txid argument
getraw <txid>

# Fetch from stdin
echo <txid> | getraw

# Fetch testnet transaction
getraw <txid> -t

# Chain with prettytx
getraw <txid> | prettytx
```

#### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--txid` | `-i` | Transaction ID to retrieve | - |
| `--testnet` | `-t` | Use testnet instead of mainnet | false |

---

## Configuration

### broadcast Configuration

Create `config.yaml` in the same directory as the `broadcast` executable:

```yaml
# Mainnet ARC endpoint
arc-mainnet:
  url: "https://api.taal.com"
  api_key: "mainnet_your_api_key_here"
  timeout: "30s"

# Testnet ARC endpoint
arc-testnet:
  url: "https://arc-test.taal.com"
  api_key: "testnet_your_api_key_here"
  timeout: "30s"

# Polling configuration for monitoring
polling:
  interval: "3s"
  max_retries: 10
  backoff_factor: 1.5

# Target statuses
targets:
  default: "SEEN_BY_NETWORK"
  wait_for_mining: false
```

**Configuration Location**:
1. First checks executable directory
2. Falls back to current working directory

---

## Examples

### Complete Transaction Workflow

```bash
# 1. Create a transaction
carve -w cR3cH1QP4e4njLNK6vaawMYPAn9bQ4YJRotkzPEVTwYsKeFfX7mT \
      -a 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa \
      -s 1000 > tx.hex

# 2. View the transaction
cat tx.hex | prettytx

# 3. Broadcast to network
cat tx.hex | broadcast -m

# 4. Check transaction later
getraw <txid> | prettytx
```

### Pipeline Example

```bash
# Create, view, and broadcast in one command
carve -w <WIF> -a <address> -s 5000 | tee >(prettytx) | broadcast -m
```

### Debug Transaction Creation

```bash
# See detailed UTXO selection and fee calculation
carve -w <WIF> -a <address> -s 1000 --debug
```

Output:
```
2024/01/15 10:30:00 Testnet: false
2024/01/15 10:30:00 Source address: 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa
2024/01/15 10:30:00 Fetching UTXOs from WhatsOnChain (main network)...
2024/01/15 10:30:00   UTXO 1 (confirmed): abc123...:0 = 5000 satoshis
2024/01/15 10:30:00   UTXO 2 (confirmed): def456...:1 = 3000 satoshis
2024/01/15 10:30:00 Found 2 UTXO(s)
2024/01/15 10:30:00 Selected 1 UTXO(s) totaling 5000 satoshis (target: 1000 + fee: ~100)
2024/01/15 10:30:00 Total input: 5000 satoshis
2024/01/15 10:30:00 Output to 1B2zP...: 1000 satoshis
2024/01/15 10:30:00 Estimated size: 226 bytes, Fee: 100 satoshis
2024/01/15 10:30:00 Change to 1A1zP...: 3900 satoshis
2024/01/15 10:30:00 Transaction ID: xyz789...
010000...
```

### Testing with Testnet

```bash
# Get testnet coins from a faucet, then:
carve -w <testnet_WIF> \
      -a mkHS9ne12qx9pS9VojpwU5xtRd4T7X7ZUt \
      -s 1000 \
      -t | broadcast -t -m
```

### Send All Funds

```bash
# Send entire balance minus fees
carve -w <WIF> -a <address> | broadcast
```

---

## Troubleshooting

### Common Issues

#### "No UTXOs found for address"

**Cause**: The source address has no unspent outputs.

**Solution**:
- Verify address has funds on WhatsOnChain
- Check you're using correct network (mainnet vs testnet)

#### "Insufficient funds"

**Cause**: Not enough satoshis to cover amount + fees.

**Solution**:
```bash
# Use --debug to see available funds
carve -w <WIF> -a <address> -s 1000 --debug
```

Then adjust the amount or fee rate.

#### "Transaction rejected by network"

**Cause**: Various reasons (double spend, invalid script, etc.)

**Solution**:
- Check transaction with `prettytx` for issues
- Verify UTXOs haven't been spent
- Ensure fee is sufficient

#### "ARC error: unauthorized"

**Cause**: Invalid or missing API key in `config.yaml`

**Solution**:
- Verify API key in `config.yaml`
- Ensure config file is in correct location
- Check you're using the right network (mainnet/testnet)

### Debug Tips

1. **Enable debug logging**: Use `--debug` flag with carve
2. **Inspect transactions**: Always pipe through `prettytx` to verify
3. **Monitor broadcasts**: Use `-m` flag to watch transaction progression
4. **Check fees**: Use `--debug` to see fee calculations
5. **Verify network**: Double-check mainnet vs testnet flags

### Getting Help

```bash
# View command help
broadcast --help
carve --help
prettytx --help
getraw --help
```

---

## Transaction Size Estimation

Understanding transaction sizes helps optimize fees:

| Component | Size (bytes) |
|-----------|--------------|
| Base overhead | ~10 |
| Per input | ~148 |
| Per output | ~34 |

**Example**: 2 inputs, 2 outputs = 10 + (2 × 148) + (2 × 34) = **374 bytes**

At 100 sat/KB fee rate: `(374 × 100) / 1000 = 37.4` → **100 sats** (minimum enforced)

---

## Security Notes

### WIF Private Keys

- **Never commit WIF keys** to version control
- **Never share WIF keys** - they control your funds
- **Use testnet** for experimentation
- **Store securely** using environment variables or secure vaults

### Best Practices

```bash
# Use environment variable for WIF
export MY_WIF="cR3cH1QP4e4njLNK6vaawMYPAn9bQ4YJRotkzPEVTwYsKeFfX7mT"
carve -w "$MY_WIF" -a <address> -s 1000

# Or read from secure file
carve -w $(cat ~/.secure/wallet.wif) -a <address> -s 1000
```

### API Keys

- **Protect ARC API keys** in `config.yaml`
- **Use different keys** for mainnet and testnet
- **Rotate keys** periodically
- **Set appropriate permissions**: `chmod 600 config.yaml`

---

## Advanced Usage

### Custom Fee Strategies

```bash
# Low priority (100 sat/KB - default)
carve -w <WIF> -a <address> -s 1000 -f 100

# Standard priority (200 sat/KB)
carve -w <WIF> -a <address> -s 1000 -f 200

# High priority (500 sat/KB)
carve -w <WIF> -a <address> -s 1000 -f 500
```

### Batch Processing

```bash
# Process multiple txids
cat txids.txt | while read txid; do
  echo "Processing $txid..."
  getraw "$txid" | prettytx > "tx_$txid.txt"
done
```

### Integration with Scripts

```bash
#!/bin/bash
# Simple payment script

WIF="$1"
DEST="$2"
AMOUNT="$3"

# Create and broadcast transaction
RAWTX=$(carve -w "$WIF" -a "$DEST" -s "$AMOUNT")

if [ $? -eq 0 ]; then
  echo "Transaction created successfully"
  echo "$RAWTX" | broadcast -m
else
  echo "Failed to create transaction" >&2
  exit 1
fi
```

---

## API Reference

### WhatsOnChain Endpoints Used

- **GET** `/v1/bsv/{network}/tx/{txid}/hex` - Get raw transaction
- **GET** `/v1/bsv/{network}/address/{address}/unspent/all` - Get all UTXOs

### ARC Endpoints Used

- **POST** `/v1/tx` - Broadcast transaction
- **GET** `/v1/tx/{txid}` - Get transaction status

---

## Version Information

- BSV SDK: `github.com/bsv-blockchain/go-sdk`
- WhatsOnChain Client: `github.com/mrz1836/go-whatsonchain`
- CLI Framework: `github.com/spf13/cobra`

---

## License

See project LICENSE file for details.
