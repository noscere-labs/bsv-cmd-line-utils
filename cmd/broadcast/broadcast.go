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

type ARCConfig struct {
	URL     string `yaml:"url"`
	APIKey  string `yaml:"api_key"`
	Timeout string `yaml:"timeout"`
}

type PollingConfig struct {
	Interval      string  `yaml:"interval"`
	MaxRetries    int     `yaml:"max_retries"`
	BackoffFactor float64 `yaml:"backoff_factor"`
}

type TargetsConfig struct {
	Default        string `yaml:"default"`
	WaitForMining  bool   `yaml:"wait_for_mining"`
}

type Config struct {
	ARCMainnet ARCConfig     `yaml:"arc-mainnet"`
	ARCTestnet ARCConfig     `yaml:"arc-testnet"`
	Polling    PollingConfig `yaml:"polling"`
	Targets    TargetsConfig `yaml:"targets"`
}

var (
	testnet  bool
	raw      string
	monitor  bool
	pollRate int
	config   Config
)

var rootCmd = &cobra.Command{
	Use:   "broadcast",
	Short: "A simple transaction broadcaster",
	Long:  "A command line tool that broadcasts bitcoin transactions from stdin",
	Run: func(cmd *cobra.Command, args []string) {
		processInput()
	},
}

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

func init() {
	rootCmd.Flags().StringVarP(&raw, "raw", "r", "", "Raw transaction hex to broadcast")
	rootCmd.Flags().BoolVarP(&monitor, "monitor", "m", false, "Monitor transaction status until final state")
	rootCmd.Flags().IntVarP(&pollRate, "poll-rate", "p", 5, "Polling rate in seconds for monitoring (default: 5)")
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet configuration from config.yaml")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func isHex(hex string) bool {
	match, _ := regexp.MatchString("^[0-9a-fA-F]+$", hex)
	return match
}
