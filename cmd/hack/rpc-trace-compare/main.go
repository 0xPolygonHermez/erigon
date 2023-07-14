package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type OpContext struct {
	Pc      uint64   `json:"pc"`
	Op      string   `json:"op"`
	Gas     uint64   `json:"gas"`
	GasCost uint64   `json:"gasCost"`
	Depth   int      `json:"depth"`
	Stack   []string `json:"stack"`
	Refund  uint64   `json:"refund"`
}

func (oc *OpContext) cmp(b OpContext) bool {
	if oc.Pc != b.Pc ||
		oc.Op != b.Op ||
		oc.Gas != b.Gas ||
		oc.GasCost != b.GasCost ||
		oc.Depth != b.Depth ||
		oc.Refund != b.Refund ||
		len(oc.Stack) != len(b.Stack) {
		return false
	}

	for i, value := range oc.Stack {
		value2 := b.Stack[i]

		if value != value2 {
			return false
		}
	}

	return true
}

var localTraceFile = "localOpDump.json"
var rpcTraceFile = "rpcOpDump.json"

func main() {
	jsonFile, err := os.Open(localTraceFile)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()

	var localTrace []OpContext
	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &localTrace)
	if err != nil {
		fmt.Println("Error parsing JSON data:", err)
		return
	}

	jsonFile2, err := os.Open(rpcTraceFile)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile2.Close()

	var rpcTrace []OpContext
	byteValue2, _ := ioutil.ReadAll(jsonFile2)
	err = json.Unmarshal(byteValue2, &rpcTrace)
	if err != nil {
		fmt.Println("Error parsing JSON data2:", err)
		return
	}

	compareTraces(localTrace, rpcTrace)

	fmt.Println("Check finished.")
}

func compareTraces(localTrace, rpcTrace []OpContext) {

	localTracelen := len(localTrace)
	rpcTracelen := len(rpcTrace)
	if localTracelen != rpcTracelen {
		fmt.Printf("opcode counts mismatch. Local count: %d, RPC count: %d\n", len(localTrace), len(rpcTrace))
	}

	for i, loc := range localTrace {
		roc := rpcTrace[i]
		areEqual := loc.cmp(roc)

		if !areEqual {
			fmt.Printf("opcodes at index {%d} are not equal.\nLocal:\t%v\nRPC:\t%v\n\n", i, loc, roc)
			return
		}
	}
}
