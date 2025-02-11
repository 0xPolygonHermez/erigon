package acl

import (
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

func TestRuleType(t *testing.T) {
	aclBoth, err := UnmarshalAcl("./test_acl_json/test-acl.json")
	if err != nil {
		t.Fatal(err)
	}
	expectedRuleBoth := AllowAll
	assert.NoError(t, err)
	assert.NotNil(t, aclBoth)
	assert.Equal(t, expectedRuleBoth, aclBoth.RuleType())

	aclDenyOnly, err := UnmarshalAcl("./test_acl_json/test-acl-deny-only.json")
	if err != nil {
		t.Fatal(err)
	}
	expectedRuleDenyOnly := AllowAll
	assert.NoError(t, err)
	assert.NotNil(t, aclDenyOnly)
	assert.Equal(t, expectedRuleDenyOnly, aclDenyOnly.RuleType())

	aclAllowOnly, err := UnmarshalAcl("./test_acl_json/test-acl-allow-only.json")
	if err != nil {
		t.Fatal(err)
	}
	expectedRuleAllowOnly := BlockAll
	assert.NoError(t, err)
	assert.NotNil(t, aclAllowOnly)
	assert.Equal(t, expectedRuleAllowOnly, aclAllowOnly.RuleType())
}
