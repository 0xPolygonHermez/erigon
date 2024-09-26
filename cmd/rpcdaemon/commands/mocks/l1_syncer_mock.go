// Code generated by MockGen. DO NOT EDIT.
// Source: /home/rr/Documentos/Iden3/cdk-erigon/zk/syncer/l1_syncer.go
//
// Generated by this command:
//
//	mockgen -source /home/rr/Documentos/Iden3/cdk-erigon/zk/syncer/l1_syncer.go -destination /home/rr/Documentos/Iden3/cdk-erigon/cmd/rpcdaemon/commands/mock/l1_syncer_mock.go -package=mocks
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	big "math/big"
	reflect "reflect"

	common "github.com/ledgerwatch/erigon-lib/common"
	ethereum "github.com/ledgerwatch/erigon"
	types "github.com/ledgerwatch/erigon/core/types"
	gomock "go.uber.org/mock/gomock"
)

// MockIEtherman is a mock of IEtherman interface.
type MockIEtherman struct {
	ctrl     *gomock.Controller
	recorder *MockIEthermanMockRecorder
}

// MockIEthermanMockRecorder is the mock recorder for MockIEtherman.
type MockIEthermanMockRecorder struct {
	mock *MockIEtherman
}

// NewMockIEtherman creates a new mock instance.
func NewMockIEtherman(ctrl *gomock.Controller) *MockIEtherman {
	mock := &MockIEtherman{ctrl: ctrl}
	mock.recorder = &MockIEthermanMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIEtherman) EXPECT() *MockIEthermanMockRecorder {
	return m.recorder
}

// BlockByNumber mocks base method.
func (m *MockIEtherman) BlockByNumber(ctx context.Context, blockNumber *big.Int) (*types.Block, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BlockByNumber", ctx, blockNumber)
	ret0, _ := ret[0].(*types.Block)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BlockByNumber indicates an expected call of BlockByNumber.
func (mr *MockIEthermanMockRecorder) BlockByNumber(ctx, blockNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BlockByNumber", reflect.TypeOf((*MockIEtherman)(nil).BlockByNumber), ctx, blockNumber)
}

// CallContract mocks base method.
func (m *MockIEtherman) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CallContract", ctx, msg, blockNumber)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CallContract indicates an expected call of CallContract.
func (mr *MockIEthermanMockRecorder) CallContract(ctx, msg, blockNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CallContract", reflect.TypeOf((*MockIEtherman)(nil).CallContract), ctx, msg, blockNumber)
}

// CallContract indicates an expected call of CallContract.
func (m *MockIEtherman) StorageAt(ctx context.Context, contract common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StorageAt", ctx, contract, key, blockNumber)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CallContract indicates an expected call of CallContract.
func (mr *MockIEthermanMockRecorder) StorageAt(ctx, contract, key, blockNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StorageAt", reflect.TypeOf((*MockIEtherman)(nil).StorageAt), ctx, contract, key, blockNumber)
}


// FilterLogs mocks base method.
func (m *MockIEtherman) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FilterLogs", ctx, query)
	ret0, _ := ret[0].([]types.Log)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FilterLogs indicates an expected call of FilterLogs.
func (mr *MockIEthermanMockRecorder) FilterLogs(ctx, query any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FilterLogs", reflect.TypeOf((*MockIEtherman)(nil).FilterLogs), ctx, query)
}

// HeaderByNumber mocks base method.
func (m *MockIEtherman) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HeaderByNumber", ctx, blockNumber)
	ret0, _ := ret[0].(*types.Header)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HeaderByNumber indicates an expected call of HeaderByNumber.
func (mr *MockIEthermanMockRecorder) HeaderByNumber(ctx, blockNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HeaderByNumber", reflect.TypeOf((*MockIEtherman)(nil).HeaderByNumber), ctx, blockNumber)
}

// TransactionByHash mocks base method.
func (m *MockIEtherman) TransactionByHash(ctx context.Context, hash common.Hash) (types.Transaction, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TransactionByHash", ctx, hash)
	ret0, _ := ret[0].(types.Transaction)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// TransactionByHash indicates an expected call of TransactionByHash.
func (mr *MockIEthermanMockRecorder) TransactionByHash(ctx, hash any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TransactionByHash", reflect.TypeOf((*MockIEtherman)(nil).TransactionByHash), ctx, hash)
}

// TransactionReceipt mocks base method.
func (m *MockIEtherman) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TransactionReceipt", ctx, txHash)
	ret0, _ := ret[0].(*types.Receipt)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// TransactionReceipt indicates an expected call of TransactionReceipt.
func (mr *MockIEthermanMockRecorder) TransactionReceipt(ctx, txHash any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TransactionReceipt", reflect.TypeOf((*MockIEtherman)(nil).TransactionReceipt), ctx, txHash)
}
