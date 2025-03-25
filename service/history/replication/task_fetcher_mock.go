// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// Code generated by MockGen. DO NOT EDIT.
// Source: task_fetcher.go
//
// Generated by this command:
//
//	mockgen -copyright_file ../../../LICENSE -package replication -source task_fetcher.go -destination task_fetcher_mock.go
//

// Package replication is a generated GoMock package.
package replication

import (
	reflect "reflect"

	quotas "go.temporal.io/server/common/quotas"
	gomock "go.uber.org/mock/gomock"
)

// MockTaskFetcherFactory is a mock of TaskFetcherFactory interface.
type MockTaskFetcherFactory struct {
	ctrl     *gomock.Controller
	recorder *MockTaskFetcherFactoryMockRecorder
	isgomock struct{}
}

// MockTaskFetcherFactoryMockRecorder is the mock recorder for MockTaskFetcherFactory.
type MockTaskFetcherFactoryMockRecorder struct {
	mock *MockTaskFetcherFactory
}

// NewMockTaskFetcherFactory creates a new mock instance.
func NewMockTaskFetcherFactory(ctrl *gomock.Controller) *MockTaskFetcherFactory {
	mock := &MockTaskFetcherFactory{ctrl: ctrl}
	mock.recorder = &MockTaskFetcherFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTaskFetcherFactory) EXPECT() *MockTaskFetcherFactoryMockRecorder {
	return m.recorder
}

// GetOrCreateFetcher mocks base method.
func (m *MockTaskFetcherFactory) GetOrCreateFetcher(clusterName string) taskFetcher {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrCreateFetcher", clusterName)
	ret0, _ := ret[0].(taskFetcher)
	return ret0
}

// GetOrCreateFetcher indicates an expected call of GetOrCreateFetcher.
func (mr *MockTaskFetcherFactoryMockRecorder) GetOrCreateFetcher(clusterName any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrCreateFetcher", reflect.TypeOf((*MockTaskFetcherFactory)(nil).GetOrCreateFetcher), clusterName)
}

// Start mocks base method.
func (m *MockTaskFetcherFactory) Start() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Start")
}

// Start indicates an expected call of Start.
func (mr *MockTaskFetcherFactoryMockRecorder) Start() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockTaskFetcherFactory)(nil).Start))
}

// Stop mocks base method.
func (m *MockTaskFetcherFactory) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop.
func (mr *MockTaskFetcherFactoryMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockTaskFetcherFactory)(nil).Stop))
}

// MocktaskFetcher is a mock of taskFetcher interface.
type MocktaskFetcher struct {
	ctrl     *gomock.Controller
	recorder *MocktaskFetcherMockRecorder
	isgomock struct{}
}

// MocktaskFetcherMockRecorder is the mock recorder for MocktaskFetcher.
type MocktaskFetcherMockRecorder struct {
	mock *MocktaskFetcher
}

// NewMocktaskFetcher creates a new mock instance.
func NewMocktaskFetcher(ctrl *gomock.Controller) *MocktaskFetcher {
	mock := &MocktaskFetcher{ctrl: ctrl}
	mock.recorder = &MocktaskFetcherMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocktaskFetcher) EXPECT() *MocktaskFetcherMockRecorder {
	return m.recorder
}

// Stop mocks base method.
func (m *MocktaskFetcher) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop.
func (mr *MocktaskFetcherMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MocktaskFetcher)(nil).Stop))
}

// getRateLimiter mocks base method.
func (m *MocktaskFetcher) getRateLimiter() quotas.RateLimiter {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getRateLimiter")
	ret0, _ := ret[0].(quotas.RateLimiter)
	return ret0
}

// getRateLimiter indicates an expected call of getRateLimiter.
func (mr *MocktaskFetcherMockRecorder) getRateLimiter() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getRateLimiter", reflect.TypeOf((*MocktaskFetcher)(nil).getRateLimiter))
}

// getRequestChan mocks base method.
func (m *MocktaskFetcher) getRequestChan() chan<- *replicationTaskRequest {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getRequestChan")
	ret0, _ := ret[0].(chan<- *replicationTaskRequest)
	return ret0
}

// getRequestChan indicates an expected call of getRequestChan.
func (mr *MocktaskFetcherMockRecorder) getRequestChan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getRequestChan", reflect.TypeOf((*MocktaskFetcher)(nil).getRequestChan))
}

// getSourceCluster mocks base method.
func (m *MocktaskFetcher) getSourceCluster() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getSourceCluster")
	ret0, _ := ret[0].(string)
	return ret0
}

// getSourceCluster indicates an expected call of getSourceCluster.
func (mr *MocktaskFetcherMockRecorder) getSourceCluster() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getSourceCluster", reflect.TypeOf((*MocktaskFetcher)(nil).getSourceCluster))
}
