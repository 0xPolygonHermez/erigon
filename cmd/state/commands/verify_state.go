package commands

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	state2 "github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/ethclient"
	"github.com/spf13/cobra"
	"math/big"
	"sync"
	"time"
)

var (
	rpcClient = new(ethclient.Client)
)

func init() {
	withDataDir(verifyAccount)
	withGap(verifyAccount)
	withRpc(verifyAccount)
	rootCmd.AddCommand(verifyAccount)
}

var verifyAccount = &cobra.Command{
	Use:   "verifyAccount",
	Short: "verify account balance and contract status",
	PreRun: func(cmd *cobra.Command, args []string) {
		checkRPCNode()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return verifyStateLoop(chaindata)
	},
}

func checkRPCNode() {
	var err error
	rpcClient, err = ethclient.Dial(rpc)
	if err != nil {
		panic(fmt.Sprintf("check rpc node failed: %s", err))
	}
	_, err = rpcClient.ChainID(context.Background())
	if err != nil {
		panic(fmt.Sprintf("get chainid from rpc failied: %s", err))
	}
}

func verifyStateLoop(chainData string) error {

	db, err := mdbx.MustOpen(chainData).BeginRw(context.Background())
	if err != nil {
		return err
	}
	defer db.Rollback()
	psr := state2.NewPlainStateReader(db)

	accList := make(map[string]*accounts.Account, 0)
	cnt := 0
	err = psr.ForEach(kv.PlainState, nil, func(k, acc []byte) error {
		cnt++
		if cnt%5000000 == 0 {
			fmt.Println("range count", cnt, "acc_size", len(accList))
		}
		if len(k) == 20 {
			a := &accounts.Account{}
			if err := a.DecodeForStorage(acc); err != nil {
				panic(err)
			}
			accList[hex.EncodeToString(k)] = a
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	currentBlockNumber, err := stages.GetStageProgress(db, stages.Execution)
	if err != nil {
		panic(err)
	}
	fmt.Println("begin to range PlainState", "accSize:", len(accList), "currentBlockNumber", currentBlockNumber)

	mainLoop(db, accList, currentBlockNumber)
	return nil
}

type task struct {
	checkBalance bool
	height       uint64
	addr         libcommon.Address
	balance      *big.Int
	storageKey   libcommon.Hash
	storageValue *big.Int
}

func getBalance(addr libcommon.Address, height uint64) *uint256.Int {
	cnt := 0
	for cnt < 20 {
		remoteBalance, e := rpcClient.BalanceAt(context.Background(), addr, new(big.Int).SetUint64(height))
		if e != nil {
			fmt.Println("getBalance err", e)
			time.Sleep(10 * time.Second)
			cnt++
		} else {
			return remoteBalance
		}
	}
	panic("getBalance failed")
}

func getStorageAt(addr libcommon.Address, storageKey libcommon.Hash, height uint64) []byte {
	cnt := 0
	for cnt < 20 {
		remoteValue, e := rpcClient.StorageAt(context.Background(), addr, storageKey, new(big.Int).SetUint64(height))
		if e != nil {
			fmt.Println("getStorageAt err", e)
			time.Sleep(10 * time.Second)
			cnt++
		} else {
			return remoteValue
		}
	}
	panic("getStorageAt failed")

}

func verifyAccountWorker(processChan chan<- struct{}, ch <-chan task, wg *sync.WaitGroup) {
	defer wg.Done()
	for true {
		t, ok := <-ch
		if !ok {
			return
		}

		processChan <- struct{}{}
		continue //todo : fake verification mode.
		//  First test whether the data acquisition is normal, and then verify the correctness of the data.

		if t.checkBalance {
			remoteBalance := getBalance(t.addr, t.height)
			if t.balance.Cmp(remoteBalance.ToBig()) != 0 {
				fmt.Println("checkBalance failed : height", t.height, "addr", t.addr.String(), "remoteBalance", remoteBalance.Uint64(), "localBalance", t.balance)
				panic("check balance failed")
			}
		} else {
			remoteValue := getStorageAt(t.addr, t.storageKey, t.height)
			if new(big.Int).SetBytes(remoteValue).Cmp(t.storageValue) != 0 {
				fmt.Println("checkStorage failed: height", t.height, "addr", t.addr, "storageKey", t.storageKey, "remoteValue", new(big.Int).SetBytes(remoteValue), "localValue", t.storageValue.String())
				panic("check balance failed")
			}
		}
	}
}

func mainLoop(db kv.RwTx, accList map[string]*accounts.Account, currentHeight uint64) {
	h := currentHeight - 1
	for true {
		fmt.Println("handle height", h, "gap", verifyStateGap)

		taskList := make([]task, 0)
		oldState := state2.NewPlainState(db, h+1, nil)

		addrCnt := 0
		for addr, _ := range accList {
			addrCnt++
			if addrCnt%100000 == 0 {
				fmt.Println("handle account index", addrCnt)
			}
			address := libcommon.HexToAddress(addr)

			err := oldState.ForEachStorage(address, libcommon.Hash{}, func(key, seckey libcommon.Hash, value uint256.Int) bool {
				taskList = append(taskList, task{
					checkBalance: false,
					height:       h,
					addr:         address,
					balance:      new(big.Int),
					storageKey:   key,
					storageValue: value.ToBig(),
				})

				return true
			}, 100000)
			if err != nil {
				panic(err)
			}

			localValue, err := oldState.ReadAccountData(address)
			if err != nil {
				panic(err)
			}
			a1Value := new(big.Int)
			if localValue != nil {
				a1Value = localValue.Balance.ToBig()
			}
			taskList = append(taskList, task{
				checkBalance: true,
				height:       h,
				addr:         address,
				balance:      a1Value,
			})

		}

		ch := make(chan task, 100000)
		var wg sync.WaitGroup
		processChan, stop := progressPrinter("verifyState", uint64(len(taskList)))
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go verifyAccountWorker(processChan, ch, &wg)
		}
		for i, t := range taskList {
			ch <- t
			if i%1000000 == 0 {
				fmt.Println("send task to chan", i)
			}
		}
		close(ch)
		wg.Wait()
		stop()

		if h > verifyStateGap {
			h -= verifyStateGap
		} else {
			return
		}
	}
}
