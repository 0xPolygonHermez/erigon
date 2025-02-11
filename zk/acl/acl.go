package acl

import (
	"encoding/json"
	"github.com/ledgerwatch/erigon-lib/common"
	"io"
	"os"
)

const (
	AllowAll RuleType = iota
	BlockAll
	Disabled
)

type RuleType uint8

func (r RuleType) String() string {
	switch r {
	case AllowAll:
		return "allowlist"
	case BlockAll:
		return "blocklist"
	case Disabled:
		return "disabled"
	default:
		return "disabled"
	}
}

type Acl struct {
	Allow *Rules `json:"allow"`
	Deny  *Rules `json:"deny"`
}

type Rules struct {
	Deploy []common.Address `json:"deploy"`
	Send   []common.Address `json:"send"`
}

func UnmarshalAcl(path string) (*Acl, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var acl Acl
	if err = json.Unmarshal(data, &acl); err != nil {
		return nil, err
	}

	return &acl, nil
}

func (a *Acl) AllowExists() bool {
	if a.Allow == nil {
		return false
	}
	return len(a.Allow.Deploy) > 0 || len(a.Allow.Send) > 0
}

func (a *Acl) DenyExists() bool {
	if a.Deny == nil {
		return false
	}
	return len(a.Deny.Deploy) > 0 || len(a.Deny.Send) > 0
}

func (a *Acl) RuleType() RuleType {
	if a.AllowExists() && !a.DenyExists() {
		return BlockAll
	}
	if !a.AllowExists() && a.DenyExists() {
		return AllowAll
	}
	if a.AllowExists() && a.DenyExists() {
		return AllowAll
	}
	return Disabled
}
