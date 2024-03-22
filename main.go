package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/bnb-chain/bcfusion/contracts"
	"github.com/bnb-chain/go-sdk/client/rpc"
	ctypes "github.com/bnb-chain/go-sdk/common/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"golang.org/x/net/context"
)

const bcNodeAddr = "tcp://dataseed1.bnbchain.org:80"
const bscNodeAddr = "https://bsc-dataseed2.bnbchain.org"
const startIndicator = "<!-- AUTO_UPDATE_START -->"
const endIndicator = "<!-- AUTO_UPDATE_END -->"
const bcPegAccount = "bnb1v8vkkymvhe2sf7gd2092ujc6hweta38xadu2pj"
const bscTokenHub = "0x0000000000000000000000000000000000001004"

func main() {
	// update readme to track token bind status
	result := getTokenBindStatus()
	fmt.Println(result)
	updateReadme(result)

	// take snapshot of token migration progress
	now := time.Now()
	snapshot := takeTokenMigrationSnapshot()
	WriteCSV(fmt.Sprintf("token_migration_progress/snapshot_%s.csv", now.Format("2006-01-02")), snapshot)
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
	cannotBindTokens := make(map[string]struct{})
	cannotBindTokens["WINB-41F"] = struct{}{}
	cannotBindTokens["TUSDB-888"] = struct{}{}
	cannotBindTokens["TRXB-2E6"] = struct{}{}
	cannotBindTokens["IDRTB-178"] = struct{}{}

	client := rpc.NewRPCClient(bcNodeAddr, ctypes.ProdNetwork)
	tokens, err := client.ListAllTokens(0, 10000)
	if err != nil {
		panic(err)
	}
	result := ""
	for _, token := range tokens {
		_, cannotBind := cannotBindTokens[token.Symbol]
		if token.ContractAddress != "" && token.Symbol != "BNB" && !cannotBind {
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

type SnapshotToken struct {
	Name            string
	Symbol          string
	ContractAddress string
	ContractDecimal int8
	TotalSupply     int64
	PegOnBC         int64
	PegOnBSC        *big.Int
}

func takeTokenMigrationSnapshot() []*SnapshotToken {
	client := rpc.NewRPCClient(bcNodeAddr, ctypes.ProdNetwork)
	tokens, err := client.ListAllTokens(0, 10000)
	if err != nil {
		panic(err)
	}

	snapshotTokens := make([]*SnapshotToken, 0)

	for _, token := range tokens {
		if token.ContractAddress != "" {
			snapshotTokens = append(snapshotTokens, &SnapshotToken{
				Name:            token.Name,
				Symbol:          token.Symbol,
				ContractAddress: token.ContractAddress,
				ContractDecimal: token.ContractDecimals,
				TotalSupply:     token.TotalSupply.ToInt64(),
				PegOnBC:         0,
				PegOnBSC:        &big.Int{},
			})
		}
	}

	pegAccount, err := types.AccAddressFromBech32(bcPegAccount)
	if err != nil {
		panic(err)
	}
	rpcClient, err := ethrpc.DialContext(context.Background(), bscNodeAddr)
	if err != nil {
		panic(err)
	}
	bscClient := ethclient.NewClient(rpcClient)

	for _, token := range snapshotTokens {
		balance, err := client.GetBalance(pegAccount, token.Symbol)
		if err != nil {
			panic(err)
		}
		token.PegOnBC = balance.Free.ToInt64()

		if token.Symbol == "BNB" {
			balance, err := bscClient.BalanceAt(context.Background(), common.HexToAddress(bscTokenHub), nil)
			if err != nil {
				fmt.Printf("Error getting BSC balance for %s: %s\n", token.Symbol, err.Error())
				continue
			}
			token.PegOnBSC = balance
		} else {
			bep20Instance, err := contracts.NewBep20(common.HexToAddress(token.ContractAddress), bscClient)
			amount, err := bep20Instance.BalanceOf(nil, common.HexToAddress(bscTokenHub))
			if err != nil {
				fmt.Printf("Error getting BSC balance for %s: %s\n", token.Symbol, err.Error())
				continue
			}
			token.PegOnBSC = amount
		}

		fmt.Printf("%s: BC: %d, BSC: %s\n", token.Symbol, token.PegOnBC, token.PegOnBSC.String())
	}
	return snapshotTokens
}

func WriteCSV(fname string, snapshotTokens []*SnapshotToken) {
	f, err := os.Create(fname)
	if err != nil {
		panic(err)
	}

	records := make([][]string, 0)
	records = append(records, []string{"Name", "Symbol", "ContractAddress", "ContractDecimal", "TotalSupply", "PegOnBC", "PegOnBSC"})
	for _, token := range snapshotTokens {
		records = append(records, []string{token.Name, token.Symbol,
			token.ContractAddress, fmt.Sprintf("%d", token.ContractDecimal),
			fmt.Sprintf("%d", token.TotalSupply),
			fmt.Sprintf("%d", token.PegOnBC), token.PegOnBSC.String()})
	}
	w := csv.NewWriter(f)
	if err = w.WriteAll(records); err != nil {
		_ = f.Close()
		panic(err)
	}
	_ = f.Close()
}
