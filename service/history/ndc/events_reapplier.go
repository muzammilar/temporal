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

//go:generate mockgen -copyright_file ../../../LICENSE -package $GOPACKAGE -source $GOFILE -destination events_reapplier_mock.go

package ndc

import (
	"context"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/serviceerror"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/service/history/hsm"
	historyi "go.temporal.io/server/service/history/interfaces"
	"go.temporal.io/server/service/history/workflow/update"
)

type (
	EventsReapplier interface {
		ReapplyEvents(
			ctx context.Context,
			ms historyi.MutableState,
			updateRegistry update.Registry,
			historyEvents []*historypb.HistoryEvent,
			runID string,
		) ([]*historypb.HistoryEvent, error)
	}

	EventsReapplierImpl struct {
		stateMachineRegistry *hsm.Registry
		metricsHandler       metrics.Handler
		logger               log.Logger
	}
)

func NewEventsReapplier(
	stateMachineRegistry *hsm.Registry,
	metricsHandler metrics.Handler,
	logger log.Logger,
) *EventsReapplierImpl {

	return &EventsReapplierImpl{
		stateMachineRegistry: stateMachineRegistry,
		metricsHandler:       metricsHandler,
		logger:               logger,
	}
}

func (r *EventsReapplierImpl) ReapplyEvents(
	ctx context.Context,
	ms historyi.MutableState,
	updateRegistry update.Registry,
	historyEvents []*historypb.HistoryEvent,
	runID string,
) ([]*historypb.HistoryEvent, error) {
	// sanity check workflow still running
	if !ms.IsWorkflowExecutionRunning() {
		return nil, serviceerror.NewInternal("unable to reapply events to closed workflow.")
	}
	reappliedEvents, err := reapplyEvents(ctx, ms, updateRegistry, r.stateMachineRegistry, historyEvents, nil, runID, false)
	if err != nil {
		return nil, err
	}
	if len(reappliedEvents) == 0 {
		return nil, nil
	}

	if !ms.IsWorkflowExecutionRunning() {
		// workflow is closed after reapplying events, no need to schedule workflow task
		return reappliedEvents, nil
	}

	// After reapply event, checking if we should schedule a workflow task
	if ms.IsWorkflowPendingOnWorkflowTaskBackoff() {
		// Do not create workflow task when the workflow has first workflow task backoff and execution is not started yet
		return reappliedEvents, nil
	}

	if !ms.HasPendingWorkflowTask() {
		if _, err := ms.AddWorkflowTaskScheduledEvent(
			false,
			enumsspb.WORKFLOW_TASK_TYPE_NORMAL,
		); err != nil {
			return nil, err
		}
	}
	return reappliedEvents, nil
}
