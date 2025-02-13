package acl

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUnmarshalAcl(t *testing.T) {
	a, err := UnmarshalAcl("./test_acl_json/test-acl.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.NotNil(t, a)
	assert.NotNil(t, a.Allow)
	assert.NotNil(t, a.Allow.Deploy)
	assert.NotNil(t, a.Allow.Send)
	assert.NotNil(t, a.Deny)
	assert.NotNil(t, a.Deny.Deploy)
	assert.NotNil(t, a.Deny.Send)
}

func TestCheckAllow(t *testing.T) {
	a, err := UnmarshalAcl("./test_acl_json/test-acl-allow-only.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.NotNil(t, a)
	assert.True(t, a.AllowExists())
	assert.False(t, a.DenyExists())
}

func TestCheckDeny(t *testing.T) {
	a, err := UnmarshalAcl("./test_acl_json/test-acl-deny-only.json")
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.NotNil(t, a)
	assert.True(t, a.DenyExists())
	assert.False(t, a.AllowExists())
}

func TestRuleTypeBlockAll(t *testing.T) {
	acl, err := UnmarshalAcl("./test_acl_json/test-acl.json")
	if err != nil {
		t.Fatal(err)
	}
	assert.NoError(t, err)
	assert.NotNil(t, acl)

	// Address is present in both so we should deny (deny trumps allow)
	v := NewPolicyValidator(acl)
	allowed, err := v.IsActionAllowed(context.TODO(), common.HexToAddress("0x0000000000000000000000000000000000000000"), 0)
	assert.NoError(t, err)
	assert.False(t, allowed)
}
