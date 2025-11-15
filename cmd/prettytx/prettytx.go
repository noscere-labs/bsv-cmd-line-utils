package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/spf13/cobra"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
)

var (
	raw      string
	noColor  bool
)

var rootCmd = &cobra.Command{
	Use:   "prettytx",
	Short: "Parse and display Bitcoin transaction components",
	Long:  "A command line tool that parses raw Bitcoin transactions and displays their components in human-readable format",
	Run: func(cmd *cobra.Command, args []string) {
		processInput()
	},
}

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

func c(color, text string) string {
	if noColor {
		return text
	}
	return color + text + colorReset
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

func isHex(hexStr string) bool {
	match, _ := regexp.MatchString("^[0-9a-fA-F]+$", hexStr)
	return match
}

func init() {
	rootCmd.Flags().StringVarP(&raw, "raw", "r", "", "Raw transaction hex to parse")
	rootCmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
