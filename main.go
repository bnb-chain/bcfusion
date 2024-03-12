package main

import (
	"bufio"
	"fmt"
	"github.com/bnb-chain/go-sdk/client/rpc"
	ctypes "github.com/bnb-chain/go-sdk/common/types"
	"io/ioutil"
	"os"
	"strings"
)

const nodeAddr = "tcp://dataseed1.bnbchain.org:80"
const startIndicator = "<!-- AUTO_UPDATE_START -->"
const endIndicator = "<!-- AUTO_UPDATE_END -->"

func main() {
	result := getTokenBindStatus()
	fmt.Println(result)

	updateReadme(result)
}

func updateReadme(result string) {
	file, err := os.Open("README.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	original := make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		original = append(original, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	current := make([]string, 0)
	replace := false
	for _, line := range original {
		if strings.HasPrefix(line, endIndicator) {
			current = append(current, "| Asset | Symbol | BSC Contract Address | Comments |")
			current = append(current, "|-|-|-|-|")
			current = append(current, result) // append result
			replace = false
		}
		if !replace {
			current = append(current, line)
		}
		if strings.HasPrefix(line, startIndicator) {
			replace = true
		}
	}

	fmt.Println("Original", strings.Join(original, "\n"))
	fmt.Println("Current", strings.Join(current, "\n"))

	err = ioutil.WriteFile("README.md", []byte(strings.Join(current, "\n")), 0644)
	if err != nil {
		panic(err)
	}
}

func getTokenBindStatus() string {
	client := rpc.NewRPCClient(nodeAddr, ctypes.ProdNetwork)
	tokens, err := client.ListAllTokens(0, 10000)
	if err != nil {
		panic(err)
	}
	result := ""
	for _, token := range tokens {
		if token.ContractAddress != "" && token.Symbol != "BNB" {
			splits := strings.Split(token.Symbol, "-")
			line := fmt.Sprintf("| %s | %s | %s | |\n", splits[0], token.Symbol, token.ContractAddress)
			result = result + line
		}
	}
	for _, token := range tokens {
		if token.ContractAddress == "" {
			splits := strings.Split(token.Symbol, "-")
			line := fmt.Sprintf("| %s | %s | | |\n", splits[0], token.Symbol)
			result = result + line
		}
	}
	return result
}
