# BSV Command Line Tools

A suite of command-line utilities for Bitcoin SV (BSV) transaction handling, building, broadcasting, and analysis.

## Tools

| Tool | Description |
|------|-------------|
| **broadcast** | Broadcasts raw transactions to the BSV network via ARC endpoints |
| **carve** | Creates and signs BSV transactions with smart UTXO selection |
| **prettytx** | Parses and displays raw transactions in human-readable format |
| **getraw** | Fetches raw transaction data from WhatsOnChain |
| **txstatus** | Checks transaction status via ARC |

## Installation

```bash
# Clone the repository
git clone https://github.com/noscere-labs/bsv-cmd-line-utils.git
cd bsv-cmd-line-utils

# Build all tools
go build ./cmd/broadcast
go build ./cmd/carve
go build ./cmd/prettytx
go build ./cmd/getraw
go build ./cmd/txstatus

# Or install globally
go install ./cmd/broadcast
go install ./cmd/carve
go install ./cmd/prettytx
go install ./cmd/getraw
go install ./cmd/txstatus
```

## Quick Start

### Create and broadcast a transaction

```bash
# Create a transaction
carve -w <WIF> -a <destination_address> -s 1000 > tx.hex

# View the transaction
cat tx.hex | prettytx

# Broadcast to network with monitoring
cat tx.hex | broadcast -m
```

### Fetch and inspect an existing transaction

```bash
getraw <txid> | prettytx
```

## Documentation

See [TOOLS.md](TOOLS.md) for comprehensive documentation including:
- Detailed usage for each tool
- Configuration options
- Examples and workflows
- Troubleshooting guide

## Configuration

The `broadcast` and `txstatus` tools require a `config.yaml` file:

```yaml
arc-mainnet:
  url: "https://api.taal.com"
  api_key: "your_api_key_here"
  timeout: "30s"

arc-testnet:
  url: "https://arc-test.taal.com"
  api_key: "your_testnet_api_key_here"
  timeout: "30s"

polling:
  interval: "3s"
  max_retries: 10
  backoff_factor: 1.5
```

## Dependencies

- [go-sdk](https://github.com/bsv-blockchain/go-sdk) - BSV SDK for Go
- [go-whatsonchain](https://github.com/mrz1836/go-whatsonchain) - WhatsOnChain API client
- [cobra](https://github.com/spf13/cobra) - CLI framework

## Project Structure

```
bsv-cmd-line-utils/
├── cmd/
│   ├── broadcast/    # Transaction broadcaster
│   ├── carve/        # Transaction builder
│   ├── getraw/       # Transaction fetcher
│   ├── prettytx/     # Transaction parser
│   └── txstatus/     # Status checker
├── internal/
│   ├── arc/          # ARC client
│   ├── cli/          # Shared CLI utilities
│   └── config/       # Configuration loading
├── TOOLS.md          # Detailed documentation
└── README.md
```

## Contributing

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for guidelines.

## Security

- Never commit WIF private keys to version control
- Protect API keys in config.yaml (`chmod 600 config.yaml`)
- Use testnet for experimentation

For security issues, see [SECURITY.md](.github/SECURITY.md).

## License

[MIT](LICENSE)

## Attribution

This project was bootstrapped using [mrz1836/go-template](https://github.com/mrz1836/go-template).
