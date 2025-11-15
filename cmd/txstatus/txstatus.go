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

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/go-template/internal/arc"
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
	Default       string `yaml:"default"`
	WaitForMining bool   `yaml:"wait_for_mining"`
}

type Config struct {
	ARCMainnet ARCConfig     `yaml:"arc-mainnet"`
	ARCTestnet ARCConfig     `yaml:"arc-testnet"`
	Polling    PollingConfig `yaml:"polling"`
	Targets    TargetsConfig `yaml:"targets"`
}

var (
	txid     string
	testnet  bool
	monitor  bool
	pollRate int
	config   Config
)

// https://arc.taal.com

var rootCmd = &cobra.Command{
	Use:   "txstatus [txid]",
	Short: "Check transaction status",
	Long:  "A command line tool that checks transaction status on ARC. Accepts txid as argument or from stdin",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var transactionID string

		// Get txid from command line argument if provided
		if len(args) > 0 {
			transactionID = args[0]
		} else if txid != "" {
			// Use flag value if provided
			transactionID = txid
		} else {
			// Check if stdin has data
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				// Data is being piped to stdin
				transactionID = readTxidFromStdin()
			}
		}

		if transactionID == "" {
			cmd.Help()
			fmt.Fprintf(os.Stderr, "\nError: no txid provided\n")
			os.Exit(1)
		}

		// Validate it's a hex string
		if !isHex(transactionID) {
			log.Fatalf("Error: txid is not a valid hex string: %s", transactionID)
		}

		checkTransactionStatus(transactionID)
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
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err = yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

func readTxidFromStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	var txidBuilder strings.Builder

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
		txidBuilder.WriteString(cleaned)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	return txidBuilder.String()
}

func checkTransactionStatus(txid string) {
	// Load configuration from config.yaml
	if err := loadConfig(); err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

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

	if monitor {
		// Continuous monitoring
		monitorTransaction(client, txid)
	} else {
		// Single status check
		getStatus(client, txid)
	}
}

func getStatus(client *arc.ARCClient, txid string) {
	fmt.Printf("Checking status for transaction: %s\n\n", txid)

	status, err := client.GetTransactionStatus(txid)
	if err != nil {
		log.Fatalf("Error getting transaction status: %v", err)
	}

	fmt.Printf("Status: %s\n", status.TxStatus)
	fmt.Printf("Description: %s\n", arc.GetStatusDescription(status.TxStatus))

	if status.ExtraInfo != "" {
		fmt.Printf("Info: %s\n", status.ExtraInfo)
	}

	if status.Timestamp != "" {
		fmt.Printf("Timestamp: %s\n", status.Timestamp)
	}

	if status.BlockHash != "" {
		fmt.Printf("Block Hash: %s\n", status.BlockHash)
		fmt.Printf("Block Height: %d\n", status.BlockHeight)
	}

	if arc.IsTransactionFinal(status.TxStatus) {
		fmt.Printf("\n✓ Transaction is in final state\n")
	} else {
		fmt.Printf("\n⏳ Transaction is still pending (use --monitor to watch for changes)\n")
	}
}

func monitorTransaction(client *arc.ARCClient, txid string) {
	fmt.Printf("Monitoring transaction: %s\n", txid)
	fmt.Printf("Polling every %d seconds...\n", pollRate)
	fmt.Println("Press Ctrl+C to stop monitoring")
	fmt.Println()

	// Do initial check immediately
	status, err := client.GetTransactionStatus(txid)
	if err != nil {
		log.Fatalf("Error getting transaction status: %v", err)
	}

	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] Status: %s - %s\n", timestamp, status.TxStatus, arc.GetStatusDescription(status.TxStatus))

	if status.BlockHash != "" {
		fmt.Printf("         Block Hash: %s\n", status.BlockHash)
		fmt.Printf("         Block Height: %d\n", status.BlockHeight)
	}

	// If already final, exit
	if arc.IsTransactionFinal(status.TxStatus) {
		fmt.Printf("\n✓ Transaction is already in final state: %s\n", status.TxStatus)
		return
	}

	// Continue monitoring
	ticker := time.NewTicker(time.Duration(pollRate) * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

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
	}
}

func init() {
	rootCmd.Flags().StringVarP(&txid, "txid", "i", "", "Transaction ID to check")
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
