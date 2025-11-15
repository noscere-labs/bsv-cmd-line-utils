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

var (
	testnet bool
	txid    string
)

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

func init() {
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet instead of mainnet")
	rootCmd.Flags().StringVarP(&txid, "txid", "i", "", "Transaction ID to retrieve")
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
