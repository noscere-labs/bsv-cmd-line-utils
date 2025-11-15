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

var (
	wif       string
	address   string
	sats      uint64
	testnet   bool
	feePerKb  uint64
	dustLimit uint64
	debug     bool
)

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

		if err := carveTransaction(); err != nil {
			log.Fatalf("Error: %v", err)
		}
	},
}

type UTXO struct {
	TxHash string `json:"tx_hash"`
	TxPos  uint32 `json:"tx_pos"`
	Value  uint64 `json:"value"`
}

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
	tx, err := buildTransaction(privKey, sourceAddress, address, selectedUTXOs, sats)
	if err != nil {
		return fmt.Errorf("failed to build transaction: %w", err)
	}

	// 5. Output the raw transaction hex to stdout
	rawHex := tx.String()
	fmt.Println(rawHex)

	return nil
}

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

// selectUTXOs selects the minimum set of UTXOs needed to cover the amount plus estimated fees
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

func buildTransaction(privKey *ec.PrivateKey, sourceAddr *script.Address, destAddrStr string, utxos []*UTXO, amount uint64) (*transaction.Transaction, error) {
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

	// Add output to destination address
	if amount > 0 {
		destLockingScript, err := p2pkh.Lock(destAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination locking script: %w", err)
		}

		tx.AddOutput(&transaction.TransactionOutput{
			Satoshis:      amount,
			LockingScript: destLockingScript,
		})

		if debug {
			log.Printf("Output to %s: %d satoshis", destAddrStr, amount)
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

func init() {
	rootCmd.Flags().StringVarP(&wif, "wif", "w", "", "Source WIF private key (required)")
	rootCmd.Flags().StringVarP(&address, "address", "a", "", "Destination address (required)")
	rootCmd.Flags().Uint64VarP(&sats, "sats", "s", 0, "Amount in satoshis to send (default: 0 = send all minus fees)")
	rootCmd.Flags().BoolVarP(&testnet, "testnet", "t", false, "Use testnet")
	rootCmd.Flags().Uint64VarP(&feePerKb, "fee-per-kb", "f", 100, "Fee per kilobyte in satoshis")
	rootCmd.Flags().Uint64VarP(&dustLimit, "dust", "d", 1, "Dust limit in satoshis")
	rootCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")

	rootCmd.MarkFlagRequired("wif")
	rootCmd.MarkFlagRequired("address")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
