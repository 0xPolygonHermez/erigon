package commands

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	utils2 "github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
	"sync"
	"time"
)

func init() {
	withDataDir(verifySmtTree)
	rootCmd.AddCommand(verifySmtTree)
}

var verifySmtTree = &cobra.Command{
	Use:   "verifySmt",
	Short: "verify all nodes of smt tree",
	RunE: func(cmd *cobra.Command, args []string) error {
		checkSMTree(chaindata)
		return nil
	},
}

func newSQLDB() (*pgxpool.Pool, error) {
	user := "state_user"
	password := "state_password"
	host := "127.0.0.1"
	port := "5432"
	name := "prover_db"

	config, err := pgxpool.ParseConfig(fmt.Sprintf("postgres://%s:%s@%s:%s/%s?pool_max_conns=%d", user, password, host, port, name, 1000))
	if err != nil {
		panic(err)
	}

	conn, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	return conn, nil
}

func transferNewKey(oldKey string) string {
	ans := "\\x"

	for index := 0; index < 64-len(oldKey); index++ {
		ans += "0"
	}
	ans += oldKey
	return ans
}

func createBuckets(chainDB kv.RwDB) {
	newTx, err := chainDB.BeginRw(context.Background())
	if err != nil {
		panic(err)
	}
	if err := db.CreateEriDbBuckets(newTx); err != nil {
		panic(err)
	}
	if err := newTx.Commit(); err != nil {
		panic(err)
	}
}

func refactorString(v string) string {
	vConc := utils2.ConvertHexToBigInt(v)
	val := utils2.ScalarToNodeValue(vConc)

	truncationLength := 12

	allFirst8PaddedWithZeros := true
	for i := 0; i < 8; i++ {
		if !strings.HasPrefix(fmt.Sprintf("%016s", val[i].Text(16)), "00000000") {
			allFirst8PaddedWithZeros = false
			break
		}
	}

	if allFirst8PaddedWithZeros {
		truncationLength = 8
	}
	ans := ""

	outputArr := make([]string, truncationLength)
	for i := 0; i < truncationLength; i++ {
		if i < len(val) {
			outputArr[i] = fmt.Sprintf("%016s", val[i].Text(16))
			ans += fmt.Sprintf("%016s", val[i].Text(16))
		} else {
			outputArr[i] = "0000000000000000"
			ans += "0000000000000000"
		}
	}
	for index := len(ans); index < 192; index++ {
		ans += "0"
	}
	return ans

}

type smtTask struct {
	k string
	v string
}

func checkSMTree(chainData string) {
	chainDB := mdbx.MustOpen(chainData)
	createBuckets(chainDB)

	tx, err := chainDB.BeginRw(context.Background())
	if err != nil {
		panic(err)
	}
	eridb := db.NewEriDb(tx)
	smt := smt.NewSMT(eridb)

	taskList := make([]smtTask, 0)
	cnt := 0
	checkValueFunc := func(k string, v string) {
		cnt++
		taskList = append(taskList, smtTask{
			k: k,
			v: v,
		})

		if cnt%1000000 == 0 {
			fmt.Println("range db index", cnt)
		}
	}
	smt.PrintDb(checkValueFunc)

	ch := make(chan smtTask, 10000000)
	var wg sync.WaitGroup

	processChan, stop := progressPrinter("verifySmt", uint64(len(taskList)))
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go smtWorker(processChan, ch, &wg)
	}
	for index, t := range taskList {
		ch <- t
		if index%1000000 == 0 {
			fmt.Println("send task to channel: index", index)
		}
	}
	close(ch)
	wg.Wait()
	stop()
}

func smtWorker(processChan chan struct{}, ch <-chan smtTask, wg *sync.WaitGroup) {
	p, _ := newSQLDB()
	defer wg.Done()
	for true {
		t, ok := <-ch
		if !ok {
			return
		}
		processChan <- struct{}{}
		continue //todo : fake verification mode.
		//  First test whether the data acquisition is normal, and then verify the correctness of the data.
		k := t.k
		v := t.v

		var value []byte
		remoteDBData := p.QueryRow(context.Background(), "select data::text from state.nodes where hash=$1", transferNewKey(k[2:]))

		if err := remoteDBData.Scan(&value); err != nil {
			if err.Error() == "no rows in result set" {
				fmt.Println("not row in result set")
				return
			} else {
				fmt.Println("failed check", k, v, hex.EncodeToString(value), remoteDBData, err.Error())
				panic(err)
			}

		}

		localValue := refactorString(v[2:])
		if localValue != string(value)[2:] {
			panic(fmt.Sprintf("failed %s %s %s %d %d", v, localValue, string(value)[2:], len(localValue), len(string(value)[2:])))
		}

	}
}

func progressPrinter(message string, total uint64) (chan struct{}, func()) {
	progress := make(chan struct{}, 10000000)
	ctDone := make(chan bool)

	go func() {
		defer close(progress)
		defer close(ctDone)

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		var pc uint64
		var pct uint64

		for {
			select {
			case <-progress:
				pc++
				if total > 0 {
					pct = (pc * 100) / total
				}
			case <-ticker.C:
				if pc > 0 {
					fmt.Printf("%s: %d/%d (%d%%)\n", message, pc, total, pct)
				}
			case <-ctDone:
				return
			}
		}
	}()

	return progress, func() { ctDone <- true }
}
