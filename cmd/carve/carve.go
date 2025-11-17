// Package main implements a Bitcoin SV transaction builder with smart UTXO selection.
//
// This tool creates and signs BSV transactions from a WIF private key, automatically
// selecting the optimal set of UTXOs to minimize transaction size while covering the
// requested amount plus fees.
//
// Features:
//   - Smart UTXO selection using largest-first algorithm
//   - Automatic fee estimation with 100 satoshi minimum floor
//   - Support for "send all" transactions (sats=0)
//   - Split payments across multiple equal outputs with remainder handling
//   - Mainnet/testnet support via WhatsOnChain API
//   - Debug mode for verbose logging
//   - Automatic change output handling with dust protection
//
// Usage:
//
//	carve -w <WIF> -a <address> -s 1000              # Send 1000 satoshis
//	carve -w <WIF> -a <address>                      # Send all funds
//	carve -w <WIF> -a <address> -s 1000 -t           # Use testnet
//	carve -w <WIF> -a <address> --debug              # Enable debug output
//	carve -w <WIF> -a <address> -f 200               # Custom fee rate
//	carve -w <WIF> -a <address> -s 1000000 -n 10     # Split 1M satoshis into 10 equal outputs
//	carve -w <WIF> -a <address> -s 1000001 -n 10     # Split into 10 outputs (9×100000 + 1×100001)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"

	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/template/p2pkh"
	"github.com/spf13/cobra"
)

// Command-line flags
var (
	wif       string // WIF private key for signing
	address   string // Destination address
	sats      uint64 // Amount to send in satoshis (0 = send all)
	split     int    // Number of outputs to split the amount into (1 = no split)
	testnet   bool   // Use testnet instead of mainnet
	feePerKb  uint64 // Fee rate in satoshis per kilobyte
	dustLimit uint64 // Minimum output value to avoid dust
	debug     bool   // Enable verbose debug logging
)

// rootCmd is the main cobra command for the carve tool.
var rootCmd = &cobra.Command{
	Use:   "carve",
	Short: "Create and sign a BSV transaction from a WIF",
	Long:  "A command line tool that creates a signed transaction from a WIF private key, sending satoshis to a destination address",
	Run: func(cmd *cobra.Command, args []string) {
		if wif == "" || address == "" {
			cmd.Help()
			fmt.Fprintf(os.Stderr, "\nError: --wif and --address are required\n")
			os.Exit(1)
		}

		if split < 1 {
			cmd.Help()
			fmt.Fprintf(os.Stderr, "\nError: --split must be at least 1\n")
			os.Exit(1)
		}

		if split > 1 && sats == 0 {
			cmd.Help()
			fmt.Fprintf(os.Stderr, "\nError: --split requires a specific amount (--sats), cannot be used with send-all mode\n")
			os.Exit(1)
		}

		if err := carveTransaction(); err != nil {
			log.Fatalf("Error: %v", err)
		}
	},
}

// UTXO represents an unspent transaction output from the WhatsOnChain API.
type UTXO struct {
	TxHash string `json:"tx_hash"` // Transaction ID containing this output
	TxPos  uint32 `json:"tx_pos"`  // Output index (vout) within the transaction
	Value  uint64 `json:"value"`   // Value in satoshis
}

// carveTransaction is the main transaction creation workflow.
// It performs the following steps:
// 1. Derives the private key and source address from the WIF
// 2. Fetches all UTXOs for the source address from WhatsOnChain
// 3. Selects the optimal set of UTXOs to cover the amount + fees
// 4. Builds and signs the transaction
// 5. Outputs the raw transaction hex to stdout
func carveTransaction() error {
	ctx := context.Background()

	// 1. Derive private key and address from WIF
	privKey, err := ec.PrivateKeyFromWif(wif)
	if err != nil {
		return fmt.Errorf("failed to parse WIF: %w", err)
	}

	if debug {
		log.Printf("Testnet: %t", testnet)
	}

	// Derive the source address from the private key
	// Note: NewAddressFromPublicKey takes mainnet bool, not testnet bool
	sourceAddress, err := script.NewAddressFromPublicKey(privKey.PubKey(), !testnet)
	if err != nil {
		return fmt.Errorf("failed to derive source address: %w", err)
	}

	if debug {
		log.Printf("Source address: %s", sourceAddress.AddressString)
	}

	// 2. Fetch UTXOs from WhatsOnChain
	utxos, err := getUnspentOutputs(ctx, sourceAddress.AddressString)
	if err != nil {
		return fmt.Errorf("failed to fetch UTXOs: %w", err)
	}

	if len(utxos) == 0 {
		return fmt.Errorf("no UTXOs found for address %s", sourceAddress.AddressString)
	}

	if debug {
		log.Printf("Found %d UTXO(s)", len(utxos))
	}

	// 3. Select appropriate UTXOs
	var selectedUTXOs []*UTXO
	if sats == 0 {
		// Send all funds - use all UTXOs
		if debug {
			log.Printf("Sending all available funds")
		}
		selectedUTXOs = utxos
	} else {
		// Select minimum UTXOs needed to cover the amount
		selectedUTXOs, err = selectUTXOs(utxos, sats, feePerKb)
		if err != nil {
			return fmt.Errorf("UTXO selection failed: %w", err)
		}
	}

	// 4. Build the transaction
	tx, err := buildTransaction(privKey, sourceAddress, address, selectedUTXOs, sats, split)
	if err != nil {
		return fmt.Errorf("failed to build transaction: %w", err)
	}

	// 5. Output the raw transaction hex to stdout
	rawHex := tx.String()
	fmt.Println(rawHex)

	return nil
}

// getUnspentOutputs fetches all unspent transaction outputs (UTXOs) for a given address
// from the WhatsOnChain API using the /unspent/all endpoint.
//
// The function:
// - Queries the appropriate network (mainnet/testnet) based on the --testnet flag
// - Filters out UTXOs that are already spent in mempool transactions
// - Returns an array of available UTXOs sorted by the API
//
// Returns an error if the API call fails or if no UTXOs are available.
func getUnspentOutputs(ctx context.Context, addr string) ([]*UTXO, error) {
	network := "main"
	if testnet {
		network = "test"
	}

	// Use direct HTTP call with /unspent/all to get all UTXOs
	url := fmt.Sprintf("https://api.whatsonchain.com/v1/bsv/%s/address/%s/unspent/all", network, addr)

	if debug {
		log.Printf("Fetching UTXOs from WhatsOnChain (%s network)...", network)
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch UTXOs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("WhatsOnChain API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	type WOCUnspent struct {
		Height              int    `json:"height"`
		TxPos               int    `json:"tx_pos"`
		TxHash              string `json:"tx_hash"`
		Value               uint64 `json:"value"`
		IsSpentInMempoolTx  bool   `json:"isSpentInMempoolTx"`
		Status              string `json:"status"`
	}

	// The /unspent/all endpoint returns an object with address, script, result array, and error
	type WOCUnspentAllResponse struct {
		Address string        `json:"address"`
		Script  string        `json:"script"`
		Result  []WOCUnspent  `json:"result"`
		Error   string        `json:"error"`
	}

	var response WOCUnspentAllResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse UTXOs: %w", err)
	}

	if response.Error != "" {
		return nil, fmt.Errorf("API error: %s", response.Error)
	}

	if len(response.Result) == 0 {
		return nil, fmt.Errorf("no UTXOs found for address %s", addr)
	}

	utxos := make([]*UTXO, 0, len(response.Result))
	for i, u := range response.Result {
		// Skip UTXOs that are already spent in mempool
		if u.IsSpentInMempoolTx {
			if debug {
				log.Printf("  UTXO %d: %s:%d = %d satoshis (skipped - spent in mempool)", i+1, u.TxHash, u.TxPos, u.Value)
			}
			continue
		}

		if debug {
			log.Printf("  UTXO %d (%s): %s:%d = %d satoshis", i+1, u.Status, u.TxHash, u.TxPos, u.Value)
		}
		utxos = append(utxos, &UTXO{
			TxHash: u.TxHash,
			TxPos:  uint32(u.TxPos),
			Value:  u.Value,
		})
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("no available UTXOs for address %s (all are spent in mempool)", addr)
	}

	return utxos, nil
}

// selectUTXOs implements a largest-first UTXO selection algorithm to minimize transaction size.
//
// The algorithm:
// 1. Sorts UTXOs by value in descending order (largest first)
// 2. Iteratively adds UTXOs until the total covers the target amount plus fees
// 3. Estimates fees dynamically as more inputs are added (148 bytes per input)
// 4. Enforces a minimum fee of 100 satoshis
//
// This approach minimizes the number of inputs required, reducing transaction size and fees.
//
// Returns the selected UTXOs or an error if insufficient funds are available.
func selectUTXOs(utxos []*UTXO, targetAmount uint64, feePerKb uint64) ([]*UTXO, error) {
	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTXOs available")
	}

	// Sort UTXOs by value (largest first) for better selection
	sortedUTXOs := make([]*UTXO, len(utxos))
	copy(sortedUTXOs, utxos)
	sort.Slice(sortedUTXOs, func(i, j int) bool {
		return sortedUTXOs[i].Value > sortedUTXOs[j].Value
	})

	var selected []*UTXO
	var totalValue uint64

	// Estimate fee per input (roughly 148 bytes per input)
	const inputSize = 148
	// Base transaction overhead + output sizes (2 outputs: payment + change, ~34 bytes each)
	const baseTxSize = 10 + 34 + 34

	for _, utxo := range sortedUTXOs {
		selected = append(selected, utxo)
		totalValue += utxo.Value

		// Calculate estimated fee with current number of inputs
		estimatedSize := uint64(len(selected)*inputSize + baseTxSize)
		estimatedFee := (estimatedSize * feePerKb) / 1000

		// Enforce minimum fee of 100 satoshis
		if estimatedFee < 100 {
			estimatedFee = 100
		}

		// Check if we have enough to cover target amount + fee
		if totalValue >= targetAmount+estimatedFee {
			if debug {
				log.Printf("Selected %d UTXO(s) totaling %d satoshis (target: %d + fee: ~%d)",
					len(selected), totalValue, targetAmount, estimatedFee)
			}
			return selected, nil
		}
	}

	// Not enough funds
	estimatedSize := uint64(len(selected)*inputSize + baseTxSize)
	estimatedFee := (estimatedSize * feePerKb) / 1000

	// Enforce minimum fee of 100 satoshis
	if estimatedFee < 100 {
		estimatedFee = 100
	}

	return nil, fmt.Errorf("insufficient funds: have %d satoshis, need %d (amount: %d + fee: ~%d)",
		totalValue, targetAmount+estimatedFee, targetAmount, estimatedFee)
}

// buildTransaction constructs and signs a BSV transaction from the selected UTXOs.
//
// The function:
// 1. Creates a new transaction and adds all selected UTXOs as inputs
// 2. Adds the payment output(s) to the destination address (if amount > 0)
//    - If numOutputs > 1, splits the amount equally across outputs
//    - Any remainder is added to the last output
// 3. Calculates transaction fees based on estimated size
// 4. Adds a change output back to the source address (if change > dust limit)
// 5. Signs all inputs with the provided private key
//
// Fee calculation:
//   - Estimates size: inputs*148 + outputs*34 + 10 (overhead)
//   - Applies fee rate: (size * feePerKb) / 1000
//   - Enforces minimum fee of 100 satoshis
//
// Change handling:
//   - Change below dust limit is added to the fee instead
//   - Default dust limit is 1 satoshi (configurable via --dust flag)
//
// Returns the signed transaction or an error if building/signing fails.
func buildTransaction(privKey *ec.PrivateKey, sourceAddr *script.Address, destAddrStr string, utxos []*UTXO, amount uint64, numOutputs int) (*transaction.Transaction, error) {
	// Create a new transaction
	tx := transaction.NewTransaction()

	// Parse destination address
	destAddr, err := script.NewAddressFromString(destAddrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}

	// Create P2PKH unlocker for signing
	unlocker, err := p2pkh.Unlock(privKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create unlocker: %w", err)
	}

	// Add all UTXOs as inputs
	var totalInput uint64
	for _, utxo := range utxos {
		// Create the locking script from the source address (P2PKH)
		lockingScript, err := p2pkh.Lock(sourceAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create locking script: %w", err)
		}

		// Add input
		err = tx.AddInputFrom(
			utxo.TxHash,
			utxo.TxPos,
			lockingScript.String(),
			utxo.Value,
			unlocker,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to add input: %w", err)
		}

		totalInput += utxo.Value
	}

	if debug {
		log.Printf("Total input: %d satoshis", totalInput)
	}

	// Add output(s) to destination address
	if amount > 0 {
		destLockingScript, err := p2pkh.Lock(destAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination locking script: %w", err)
		}

		if numOutputs < 1 {
			numOutputs = 1
		}

		// Calculate amount per output and remainder
		amountPerOutput := amount / uint64(numOutputs)
		remainder := amount % uint64(numOutputs)

		// Add outputs with equal amounts
		for i := 0; i < numOutputs; i++ {
			outputAmount := amountPerOutput

			// Add remainder to the last output
			if i == numOutputs-1 {
				outputAmount += remainder
			}

			tx.AddOutput(&transaction.TransactionOutput{
				Satoshis:      outputAmount,
				LockingScript: destLockingScript,
			})

			if debug {
				log.Printf("Output %d to %s: %d satoshis", i+1, destAddrStr, outputAmount)
			}
		}

		if debug && remainder > 0 {
			log.Printf("Remainder of %d satoshis added to last output", remainder)
		}
	}

	// Calculate fees and add change output
	// Estimate size: each input ~148 bytes, each output ~34 bytes, overhead ~10 bytes
	estimatedSize := uint64(len(tx.Inputs)*148 + len(tx.Outputs)*34 + 10)
	fee := (estimatedSize * feePerKb) / 1000

	// Add extra for the change output size
	fee += 34 * feePerKb / 1000

	// Enforce minimum fee of 100 satoshis
	if fee < 100 {
		fee = 100
	}

	if debug {
		log.Printf("Estimated size: %d bytes, Fee: %d satoshis", estimatedSize, fee)
	}

	change := totalInput - amount - fee

	if change > dustLimit {
		// Add change output back to source address
		changeLockingScript, err := p2pkh.Lock(sourceAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create change locking script: %w", err)
		}

		tx.AddOutput(&transaction.TransactionOutput{
			Satoshis:      change,
			LockingScript: changeLockingScript,
		})

		if debug {
			log.Printf("Change to %s: %d satoshis", sourceAddr.AddressString, change)
		}
	} else if change > 0 {
		if debug {
			log.Printf("Change (%d satoshis) below dust limit, adding to fee", change)
		}
	}

	// Sign all inputs
	if err := tx.Sign(); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	txid := tx.TxID()
	if debug {
		log.Printf("Transaction ID: %s", txid.String())
	}

	return tx, nil
}

// init initializes the cobra command flags and marks required flags.
// This function is automatically called by Go before main() executes.
func init() {
	rootCmd.Flags().StringVarP(&wif, "wif", "w", "", "Source WIF private key (required)")
	rootCmd.Flags().StringVarP(&address, "address", "a", "", "Destination address (required)")
	rootCmd.Flags().Uint64VarP(&sats, "sats", "s", 0, "Amount in satoshis to send (default: 0 = send all minus fees)")
	rootCmd.Flags().IntVarP(&split, "split", "n", 1, "Number of equal outputs to split the amount into (default: 1 = no split)")
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet")
	rootCmd.Flags().Uint64VarP(&feePerKb, "fee-per-kb", "f", 100, "Fee per kilobyte in satoshis")
	rootCmd.Flags().Uint64VarP(&dustLimit, "dust", "d", 1, "Dust limit in satoshis")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.MarkFlagRequired("wif")
	rootCmd.MarkFlagRequired("address")
}

// main is the entry point for the carve command.
// It executes the cobra root command which handles flag parsing and command execution.
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
