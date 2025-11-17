// Package main implements a Bitcoin SV transaction broadcaster using ARC (BSV Transaction Processing).
//
// This tool broadcasts raw Bitcoin transactions to the BSV network via ARC endpoints
// and optionally monitors their status until they reach a final state (MINED, REJECTED, etc.).
//
// Features:
//   - Config-based mainnet/testnet endpoint management via config.yaml
//   - Real-time transaction status monitoring with customizable polling
//   - Support for stdin or command-line input
//   - Automatic transaction lifecycle tracking
//
// Usage:
//
//	echo "010000..." | broadcast              # Broadcast from stdin
//	broadcast -r "010000..."                  # Broadcast using flag
//	broadcast -t -m                           # Testnet with monitoring
//	broadcast -m -p 10                        # Monitor with 10s poll rate
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mrz1836/go-template/internal/arc"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ARCConfig holds the configuration for an ARC endpoint (mainnet or testnet).
type ARCConfig struct {
	URL     string `yaml:"url"`      // ARC endpoint URL (e.g., "https://api.taal.com")
	APIKey  string `yaml:"api_key"`  // API key for authentication
	Timeout string `yaml:"timeout"`  // HTTP timeout duration (e.g., "30s")
}

// PollingConfig defines parameters for transaction status polling when monitoring is enabled.
type PollingConfig struct {
	Interval      string  `yaml:"interval"`       // Time between status checks (e.g., "3s")
	MaxRetries    int     `yaml:"max_retries"`    // Maximum number of retry attempts
	BackoffFactor float64 `yaml:"backoff_factor"` // Multiplier for exponential backoff
}

// TargetsConfig specifies target states for transaction monitoring.
type TargetsConfig struct {
	Default       string `yaml:"default"`          // Default target status to wait for
	WaitForMining bool   `yaml:"wait_for_mining"`  // Whether to wait for MINED status
}

// Config is the root configuration structure loaded from config.yaml.
type Config struct {
	ARCMainnet ARCConfig     `yaml:"arc-mainnet"` // Mainnet ARC configuration
	ARCTestnet ARCConfig     `yaml:"arc-testnet"` // Testnet ARC configuration
	Polling    PollingConfig `yaml:"polling"`     // Polling parameters for monitoring
	Targets    TargetsConfig `yaml:"targets"`     // Target status configuration
}

// Command-line flags and global configuration
var (
	testnet  bool   // Use testnet instead of mainnet
	raw      string // Raw transaction hex provided via flag
	monitor  bool   // Enable transaction status monitoring
	pollRate int    // Polling interval in seconds for monitoring
	config   Config // Loaded configuration from config.yaml
)

// rootCmd is the main cobra command for the broadcast tool.
var rootCmd = &cobra.Command{
	Use:   "broadcast",
	Short: "A simple transaction broadcaster",
	Long:  "A command line tool that broadcasts bitcoin transactions from stdin",
	Run: func(cmd *cobra.Command, args []string) {
		processInput()
	},
}

// loadConfig reads and parses the config.yaml file.
// It first checks the executable directory, then falls back to the current working directory.
// Returns an error if the config file cannot be found or parsed.
func loadConfig() error {
	// Get the executable directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Try config.yaml in the executable directory first
	configPath := filepath.Join(exeDir, "config.yaml")

	// If not found, try the current working directory
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// processInput handles the main execution flow:
// 1. Loads configuration from config.yaml
// 2. Reads transaction hex from flag or stdin
// 3. Validates the hex string
// 4. Broadcasts the transaction to ARC
func processInput() {
	// Load configuration from config.yaml
	if err := loadConfig(); err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	var txString string

	// Get transaction from raw flag or stdin
	if raw != "" {
		txString = raw
	} else {
		txString = readTxFromStdin()
	}

	if txString == "" {
		log.Fatal("Error: no transaction provided")
	}

	// Check the string to ensure it is a hex string
	if !isHex(txString) {
		log.Fatalf("Error: input is not a valid hex string")
	}

	fmt.Printf("Transaction hex: %s\n", txString)

	// Broadcast transaction using ARC
	broadcastTransaction(txString)
}

// readTxFromStdin reads transaction hex from stdin.
// It strips all whitespace and control characters, returning only printable ASCII characters.
// This allows for flexible input formatting (newlines, spaces, etc.).
func readTxFromStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	var txHex strings.Builder

	// Read all text via stdin into a single string with no spaces or control characters
	for scanner.Scan() {
		line := scanner.Text()
		// Remove all whitespace and control characters
		cleaned := strings.Map(func(r rune) rune {
			if r > 32 && r < 127 {
				return r
			}
			return -1
		}, line)
		txHex.WriteString(cleaned)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	return txHex.String()
}

// broadcastTransaction sends a raw transaction to the ARC network.
// It selects the appropriate endpoint (mainnet/testnet) based on the --testnet flag,
// creates an ARC client, broadcasts the transaction, and displays the result.
// If --monitor flag is set, it will continuously poll the transaction status.
func broadcastTransaction(rawTx string) {
	// Use config based on testnet flag
	var url, key string
	if testnet {
		url = config.ARCTestnet.URL
		key = config.ARCTestnet.APIKey
		fmt.Println("Using testnet configuration")
	} else {
		url = config.ARCMainnet.URL
		key = config.ARCMainnet.APIKey
		fmt.Println("Using mainnet configuration")
	}

	// Validate ARC URL is available
	if url == "" {
		log.Fatal("Error: ARC URL is required in config.yaml")
	}

	// Create ARC client
	client := arc.NewARCClient(url, key)

	fmt.Println("Broadcasting transaction to ARC...")

	// Broadcast the transaction
	resp, err := client.BroadcastTransaction(rawTx)
	if err != nil {
		log.Fatalf("Error broadcasting transaction: %v", err)
	}

	fmt.Printf("✓ Transaction broadcast successful!\n")
	fmt.Printf("  TxID: %s\n", resp.TxID)
	fmt.Printf("  Status: %s\n", resp.TxStatus)
	fmt.Printf("  Description: %s\n", arc.GetStatusDescription(resp.TxStatus))
	if resp.ExtraInfo != "" {
		fmt.Printf("  Info: %s\n", resp.ExtraInfo)
	}

	// Monitor transaction status if requested
	if monitor {
		monitorTransaction(client, resp.TxID)
	}
}

// monitorTransaction continuously polls the transaction status until it reaches a final state.
// Final states are: MINED, REJECTED, or DOUBLE_SPEND_ATTEMPTED.
// The polling interval is controlled by the --poll-rate flag (default: 5 seconds).
// Displays timestamped status updates and block information if available.
func monitorTransaction(client *arc.ARCClient, txid string) {
	fmt.Printf("\nMonitoring transaction status (polling every %d seconds)...\n", pollRate)
	fmt.Println("Press Ctrl+C to stop monitoring")
	fmt.Println()

	ticker := time.NewTicker(time.Duration(pollRate) * time.Second)
	defer ticker.Stop()

	for {
		status, err := client.GetTransactionStatus(txid)
		if err != nil {
			log.Printf("Error getting transaction status: %v", err)
			continue
		}

		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] Status: %s - %s\n", timestamp, status.TxStatus, arc.GetStatusDescription(status.TxStatus))

		if status.BlockHash != "" {
			fmt.Printf("         Block Hash: %s\n", status.BlockHash)
			fmt.Printf("         Block Height: %d\n", status.BlockHeight)
		}

		// Stop monitoring if transaction reached final state
		if arc.IsTransactionFinal(status.TxStatus) {
			fmt.Printf("\n✓ Transaction reached final state: %s\n", status.TxStatus)
			break
		}

		<-ticker.C
	}
}

// init initializes the cobra command flags.
// This function is automatically called by Go before main() executes.
func init() {
	rootCmd.Flags().StringVarP(&raw, "raw", "r", "", "Raw transaction hex to broadcast")
	rootCmd.Flags().BoolVarP(&monitor, "monitor", "m", false, "Monitor transaction status until final state")
	rootCmd.Flags().IntVarP(&pollRate, "poll-rate", "p", 5, "Polling rate in seconds for monitoring (default: 5)")
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet configuration from config.yaml")
}

// main is the entry point for the broadcast command.
// It executes the cobra root command which handles flag parsing and command execution.
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// isHex validates that a string contains only hexadecimal characters (0-9, a-f, A-F).
// Returns true if the string is valid hex, false otherwise.
func isHex(hex string) bool {
	match, _ := regexp.MatchString("^[0-9a-fA-F]+$", hex)
	return match
}
