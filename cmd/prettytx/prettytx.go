// Package main implements a Bitcoin SV transaction parser and visualizer.
//
// This tool parses raw BSV transactions and displays their components in a human-readable,
// colorized format. It breaks down the transaction structure including version, inputs,
// outputs, scripts, and locktime.
//
// Features:
//   - Colorized output for better readability (can be disabled)
//   - Detailed breakdown of all transaction components
//   - Script hex display for inputs and outputs
//   - Address extraction for P2PKH scripts (inputs and outputs)
//   - Satoshi to BSV conversion
//   - Locktime interpretation (block height vs timestamp)
//   - Support for stdin or command-line input
//
// Usage:
//
//	echo "010000..." | prettytx               # Parse from stdin
//	prettytx -r "010000..."                   # Parse using flag
//	prettytx --no-color                       # Disable colors
//	carve -w <WIF> -a <addr> | prettytx       # Chain with carve
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/spf13/cobra"
)

// ANSI color codes for terminal output styling
const (
	colorReset   = "\033[0m"  // Reset to default
	colorRed     = "\033[31m" // Red text
	colorGreen   = "\033[32m" // Green text
	colorYellow  = "\033[33m" // Yellow text
	colorBlue    = "\033[34m" // Blue text
	colorMagenta = "\033[35m" // Magenta text
	colorCyan    = "\033[36m" // Cyan text
	colorWhite   = "\033[37m" // White text
	colorBold    = "\033[1m"  // Bold text
	colorDim     = "\033[2m"  // Dimmed text
)

// Command-line flags
var (
	raw     string // Raw transaction hex provided via flag
	noColor bool   // Disable colored output
)

// rootCmd is the main cobra command for the prettytx tool.
var rootCmd = &cobra.Command{
	Use:   "prettytx",
	Short: "Parse and display Bitcoin transaction components",
	Long:  "A command line tool that parses raw Bitcoin transactions and displays their components in human-readable format",
	Run: func(cmd *cobra.Command, args []string) {
		processInput()
	},
}

// processInput handles the main execution flow:
// 1. Reads transaction hex from flag or stdin
// 2. Validates the hex string
// 3. Parses and displays the transaction
func processInput() {
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

	// Parse and display transaction
	parseTransaction(txString)
}

// c applies ANSI color codes to text if color output is enabled.
// Returns plain text if --no-color flag is set, otherwise returns colorized text.
func c(color, text string) string {
	if noColor {
		return text
	}
	return color + text + colorReset
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

// parseTransaction decodes and displays a raw Bitcoin transaction in human-readable format.
//
// Output includes:
//   - Version number (hex and decimal)
//   - Input count and detailed breakdown of each input:
//     * Previous transaction ID and output index (vout)
//     * Unlocking script length and hex
//     * Address (if P2PKH script)
//     * Sequence number (hex and decimal)
//   - Output count and detailed breakdown of each output:
//     * Value in satoshis and BSV
//     * Locking script length and hex
//     * Address (if P2PKH script)
//   - Locktime value and interpretation:
//     * 0: Not locked
//     * < 500000000: Block height
//     * >= 500000000: Unix timestamp
//   - Transaction ID (TXID)
//
// Uses color-coding for different components (configurable via --no-color flag).
func parseTransaction(rawTx string) {
	// Decode hex to bytes
	txBytes, err := hex.DecodeString(rawTx)
	if err != nil {
		log.Fatalf("Error decoding hex: %v", err)
	}

	// Parse transaction using BSV SDK
	tx, err := transaction.NewTransactionFromBytes(txBytes)
	if err != nil {
		log.Fatalf("Error parsing transaction: %v", err)
	}

	fmt.Println(c(colorBold+colorCyan, "================================================================================"))
	fmt.Println(c(colorBold+colorCyan, "TRANSACTION BREAKDOWN"))
	fmt.Println(c(colorBold+colorCyan, "================================================================================"))
	fmt.Println()

	// Version
	fmt.Printf("%s %s %s\n",
		c(colorYellow, "Version:"),
		c(colorWhite, fmt.Sprintf("%d", tx.Version)),
		c(colorDim, fmt.Sprintf("(0x%08x)", tx.Version)))
	fmt.Println()

	// Input counter
	inputCount := len(tx.Inputs)
	fmt.Printf("%s %s\n", c(colorYellow, "In-counter:"), c(colorWhite, fmt.Sprintf("%d", inputCount)))
	fmt.Println()

	// List of inputs
	if inputCount > 0 {
		fmt.Println(c(colorBold+colorGreen, "INPUTS:"))
		fmt.Println(c(colorGreen, "--------------------------------------------------------------------------------"))
		for i, input := range tx.Inputs {
			fmt.Printf("\n%s\n", c(colorBold+colorGreen, fmt.Sprintf("Input #%d:", i)))
			fmt.Println()

			// Previous transaction ID
			if input.SourceTXID != nil {
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Prev TX ID:"),
					c(colorCyan, input.SourceTXID.String()))
			} else {
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Prev TX ID:"),
					c(colorRed, "(null)"))
			}

			// Previous output index
			fmt.Printf("  %s %s\n",
				c(colorYellow, "Prev Vout:"),
				c(colorWhite, fmt.Sprintf("%d", input.SourceTxOutIndex)))

			// Script
			unlockingScript := input.UnlockingScript
			if unlockingScript != nil {
				scriptBytes := *unlockingScript
				scriptHex := scriptBytes.String()
				scriptLen := len(scriptBytes)
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script Length:"),
					c(colorWhite, fmt.Sprintf("%d bytes", scriptLen)))
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script (hex):"),
					c(colorMagenta, scriptHex))

				// Try to extract address from P2PKH unlocking script
				addr := extractAddressFromUnlockingScript(unlockingScript, true)
				if addr != "" {
					fmt.Printf("  %s %s\n",
						c(colorYellow, "Address:"),
						c(colorCyan, addr))
				}
			} else {
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script Length:"),
					c(colorWhite, "0 bytes"))
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script (hex):"),
					c(colorDim, "(empty)"))
			}

			// Sequence number
			fmt.Printf("  %s %s %s\n",
				c(colorYellow, "Sequence:"),
				c(colorWhite, fmt.Sprintf("%d", input.SequenceNumber)),
				c(colorDim, fmt.Sprintf("(0x%08x)", input.SequenceNumber)))
		}
		fmt.Println()
	}

	// Output counter
	outputCount := len(tx.Outputs)
	fmt.Printf("%s %s\n", c(colorYellow, "Out-counter:"), c(colorWhite, fmt.Sprintf("%d", outputCount)))
	fmt.Println()

	// List of outputs
	if outputCount > 0 {
		fmt.Println(c(colorBold+colorBlue, "OUTPUTS:"))
		fmt.Println(c(colorBlue, "--------------------------------------------------------------------------------"))
		for i, output := range tx.Outputs {
			fmt.Printf("\n%s\n", c(colorBold+colorBlue, fmt.Sprintf("Output #%d:", i)))
			fmt.Println()

			// Value in satoshis
			satoshis := output.Satoshis
			btc := float64(satoshis) / 100000000.0
			fmt.Printf("  %s %s %s\n",
				c(colorYellow, "Value:"),
				c(colorGreen, fmt.Sprintf("%d satoshis", satoshis)),
				c(colorDim, fmt.Sprintf("(%.8f BSV)", btc)))

			// Locking script
			lockingScript := output.LockingScript
			if lockingScript != nil {
				scriptBytes := *lockingScript
				scriptHex := scriptBytes.String()
				scriptLen := len(scriptBytes)
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script Length:"),
					c(colorWhite, fmt.Sprintf("%d bytes", scriptLen)))
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script (hex):"),
					c(colorMagenta, scriptHex))

				// Try to extract P2PKH address
				addr := extractP2PKHAddress(lockingScript, true)
				if addr != "" {
					fmt.Printf("  %s %s\n",
						c(colorYellow, "Address:"),
						c(colorCyan, addr))
				}
			} else {
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script Length:"),
					c(colorWhite, "0 bytes"))
				fmt.Printf("  %s %s\n",
					c(colorYellow, "Script (hex):"),
					c(colorDim, "(empty)"))
			}
		}
		fmt.Println()
	}

	// Locktime
	fmt.Printf("%s %s %s\n",
		c(colorYellow, "nLockTime:"),
		c(colorWhite, fmt.Sprintf("%d", tx.LockTime)),
		c(colorDim, fmt.Sprintf("(0x%08x)", tx.LockTime)))
	if tx.LockTime == 0 {
		fmt.Printf("           %s\n", c(colorDim, "(Not locked)"))
	} else if tx.LockTime < 500000000 {
		fmt.Printf("           %s\n", c(colorDim, fmt.Sprintf("(Locked until block height %d)", tx.LockTime)))
	} else {
		fmt.Printf("           %s\n", c(colorDim, fmt.Sprintf("(Locked until Unix timestamp %d)", tx.LockTime)))
	}

	fmt.Println()
	fmt.Println(c(colorBold+colorCyan, "================================================================================"))
	fmt.Printf("%s %s\n",
		c(colorBold+colorYellow, "Transaction ID:"),
		c(colorBold+colorGreen, tx.TxID().String()))
	fmt.Println(c(colorBold+colorCyan, "================================================================================"))
}

// isHex validates that a string contains only hexadecimal characters (0-9, a-f, A-F).
// Returns true if the string is valid hex, false otherwise.
func isHex(hexStr string) bool {
	match, _ := regexp.MatchString("^[0-9a-fA-F]+$", hexStr)
	return match
}

// isP2PKH checks if a locking script is a standard P2PKH (Pay-to-PubKey-Hash) script.
// P2PKH scripts have the pattern: OP_DUP OP_HASH160 <20-byte-hash> OP_EQUALVERIFY OP_CHECKSIG
// This is exactly 25 bytes: 76 a9 14 <20 bytes> 88 ac
func isP2PKH(scriptBytes *script.Script) bool {
	if scriptBytes == nil {
		return false
	}

	bytes := []byte(*scriptBytes)

	// Check length (must be exactly 25 bytes)
	if len(bytes) != 25 {
		return false
	}

	// Check the P2PKH pattern
	return bytes[0] == 0x76 && // OP_DUP
		bytes[1] == 0xa9 && // OP_HASH160
		bytes[2] == 0x14 && // Push 20 bytes
		bytes[23] == 0x88 && // OP_EQUALVERIFY
		bytes[24] == 0xac // OP_CHECKSIG
}

// extractP2PKHAddress extracts the address from a P2PKH locking script.
// Returns the address string if successful, empty string otherwise.
func extractP2PKHAddress(scriptBytes *script.Script, mainnet bool) string {
	if !isP2PKH(scriptBytes) {
		return ""
	}

	bytes := []byte(*scriptBytes)

	// Extract the 20-byte public key hash (bytes 3-22)
	pubKeyHash := bytes[3:23]

	// Create an address from the hash
	addr, err := script.NewAddressFromPublicKeyHash(pubKeyHash, mainnet)
	if err != nil {
		return ""
	}

	return addr.AddressString
}

// extractAddressFromUnlockingScript attempts to extract an address from a P2PKH unlocking script.
// P2PKH unlocking scripts contain: <signature> <pubKey>
// This function extracts the public key and derives the address from it.
// Returns the address string if successful, empty string otherwise.
func extractAddressFromUnlockingScript(scriptBytes *script.Script, mainnet bool) string {
	if scriptBytes == nil {
		return ""
	}

	bytes := []byte(*scriptBytes)
	if len(bytes) == 0 {
		return ""
	}

	// Parse the script to extract the public key
	// In a typical P2PKH unlocking script:
	// - First comes the signature (variable length, typically ~72 bytes)
	// - Then comes the public key (33 or 65 bytes)

	// We'll try to find the public key by looking for the last push operation
	// This is a simplified parser

	var pubKeyBytes []byte
	i := 0

	for i < len(bytes) {
		if i >= len(bytes) {
			break
		}

		opcode := bytes[i]
		i++

		// Handle push data opcodes
		if opcode > 0 && opcode <= 75 {
			// Direct push of N bytes
			length := int(opcode)
			if i+length > len(bytes) {
				break
			}
			data := bytes[i : i+length]
			i += length

			// Check if this looks like a public key (33 or 65 bytes)
			if length == 33 || length == 65 {
				pubKeyBytes = data
			}
		} else if opcode == 0x4c { // OP_PUSHDATA1
			if i >= len(bytes) {
				break
			}
			length := int(bytes[i])
			i++
			if i+length > len(bytes) {
				break
			}
			data := bytes[i : i+length]
			i += length

			if length == 33 || length == 65 {
				pubKeyBytes = data
			}
		}
	}

	if len(pubKeyBytes) == 0 {
		return ""
	}

	// Try to parse the public key
	pubKey, err := ec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return ""
	}

	// Derive the address from the public key
	addr, err := script.NewAddressFromPublicKey(pubKey, mainnet)
	if err != nil {
		return ""
	}

	return addr.AddressString
}

// init initializes the cobra command flags.
// This function is automatically called by Go before main() executes.
func init() {
	rootCmd.Flags().StringVarP(&raw, "raw", "r", "", "Raw transaction hex to parse")
	rootCmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
}

// main is the entry point for the prettytx command.
// It executes the cobra root command which handles flag parsing and command execution.
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
