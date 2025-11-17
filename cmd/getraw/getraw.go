// Package main implements a Bitcoin SV transaction fetcher using the WhatsOnChain API.
//
// This tool retrieves raw transaction data (hex) from the WhatsOnChain blockchain explorer.
// It supports both mainnet and testnet, and accepts transaction IDs via command-line argument,
// flag, or stdin for flexible integration with other tools.
//
// Features:
//   - Mainnet/testnet support via --testnet flag
//   - Flexible input: argument, flag, or stdin
//   - Direct integration with WhatsOnChain API
//   - Easy chaining with other tools (e.g., prettytx)
//
// Usage:
//
//	getraw <txid>                    # Fetch by argument
//	getraw -i <txid>                 # Fetch by flag
//	echo <txid> | getraw             # Fetch from stdin
//	getraw <txid> -t                 # Fetch from testnet
//	getraw <txid> | prettytx         # Chain with prettytx
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/mrz1836/go-whatsonchain"
	"github.com/spf13/cobra"
)

// Command-line flags
var (
	testnet bool   // Use testnet instead of mainnet
	txid    string // Transaction ID provided via flag
)

// rootCmd is the main cobra command for the getraw tool.
var rootCmd = &cobra.Command{
	Use:   "getraw [txid]",
	Short: "Get raw transaction data",
	Long:  "A command line tool that retrieves raw transaction data from WhatsOnChain. Accepts txid as argument or from stdin",
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

		getRawFromWhatsOnChain(transactionID)
	},
}

// readTxidFromStdin reads a transaction ID from stdin.
// It strips all whitespace and control characters, returning only printable ASCII characters.
// This allows for flexible input formatting (newlines, spaces, etc.).
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

// getRawFromWhatsOnChain fetches raw transaction data from the WhatsOnChain API.
// It creates a client for the appropriate network (mainnet/testnet) based on the
// --testnet flag, queries the API for the transaction, and prints the raw hex to stdout.
//
// Logs the chain and network information to stderr.
// Outputs the raw transaction hex to stdout for easy piping to other tools.
func getRawFromWhatsOnChain(txid string) {
	ctx := context.Background()

	var client whatsonchain.ClientInterface
	var err error

	// Create client based on testnet flag
	if testnet {
		client, err = whatsonchain.NewClient(ctx, whatsonchain.WithNetwork(whatsonchain.NetworkTest))
	} else {
		client, err = whatsonchain.NewClient(ctx, whatsonchain.WithNetwork(whatsonchain.NetworkMain))
	}

	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Chain: %s, Network: %s\n", client.Chain(), client.Network())

	// Get raw transaction data
	rawTx, err := client.GetRawTransactionData(ctx, txid)
	if err != nil {
		log.Fatalf("Error getting raw transaction: %v", err)
	}

	// Print the raw transaction hex
	fmt.Println(rawTx)
}

// init initializes the cobra command flags.
// This function is automatically called by Go before main() executes.
func init() {
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet instead of mainnet")
	rootCmd.Flags().StringVarP(&txid, "txid", "i", "", "Transaction ID to retrieve")
}

// main is the entry point for the getraw command.
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
