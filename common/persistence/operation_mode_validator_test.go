package persistence

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	enumsspb "go.temporal.io/server/api/enums/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
)

type (
	validateOperationWorkflowModeStateSuite struct {
		suite.Suite
	}
)

func TestValidateOperationWorkflowModeStateSuite(t *testing.T) {
	s := new(validateOperationWorkflowModeStateSuite)
	suite.Run(t, s)
}

func (s *validateOperationWorkflowModeStateSuite) SetupSuite() {
}

func (s *validateOperationWorkflowModeStateSuite) TearDownSuite() {

}

func (s *validateOperationWorkflowModeStateSuite) SetupTest() {

}

func (s *validateOperationWorkflowModeStateSuite) TearDownTest() {

}

func (s *validateOperationWorkflowModeStateSuite) TestCreateMode_UpdateCurrent() {

	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}

	creatModes := []CreateWorkflowMode{
		CreateWorkflowModeBrandNew,
		CreateWorkflowModeUpdateCurrent,
	}

	for state, expectError := range stateToError {
		testSnapshot := s.newTestWorkflowSnapshot(state)
		for _, createMode := range creatModes {
			err := ValidateCreateWorkflowModeState(createMode, testSnapshot)
			if !expectError {
				s.NoError(err, err)
			} else {
				s.Error(err, err)
			}
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) TestCreateMode_BypassCurrent() {

	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}

	for state, expectError := range stateToError {
		testSnapshot := s.newTestWorkflowSnapshot(state)
		err := ValidateCreateWorkflowModeState(CreateWorkflowModeBypassCurrent, testSnapshot)
		if !expectError {
			s.NoError(err, err)
		} else {
			s.Error(err, err)
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) TestUpdateMode_UpdateCurrent() {

	// only current workflow
	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	for state, expectError := range stateToError {
		testCurrentMutation := s.newTestWorkflowMutation(state)
		err := ValidateUpdateWorkflowModeState(
			UpdateWorkflowModeUpdateCurrent,
			testCurrentMutation,
			nil,
		)
		if !expectError {
			s.NoError(err, err)
		} else {
			s.Error(err, err)
		}
	}

	// current workflow & new workflow
	currentStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	newStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	for currentState, currentExpectError := range currentStateToError {
		for newState, newExpectError := range newStateToError {
			testCurrentMutation := s.newTestWorkflowMutation(currentState)
			testNewSnapshot := s.newTestWorkflowSnapshot(newState)
			err := ValidateUpdateWorkflowModeState(
				UpdateWorkflowModeUpdateCurrent,
				testCurrentMutation,
				&testNewSnapshot,
			)
			if currentExpectError || newExpectError {
				s.Error(err, err)
			} else {
				s.NoError(err, err)
			}
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) TestUpdateMode_BypassCurrent() {

	// only current workflow
	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for state, expectError := range stateToError {
		testMutation := s.newTestWorkflowMutation(state)
		err := ValidateUpdateWorkflowModeState(
			UpdateWorkflowModeBypassCurrent,
			testMutation,
			nil,
		)
		if !expectError {
			s.NoError(err, err)
		} else {
			s.Error(err, err)
		}
	}

	// current workflow & new workflow
	currentStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	newStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for currentState, currentExpectError := range currentStateToError {
		for newState, newExpectError := range newStateToError {
			testCurrentMutation := s.newTestWorkflowMutation(currentState)
			testNewSnapshot := s.newTestWorkflowSnapshot(newState)
			err := ValidateUpdateWorkflowModeState(
				UpdateWorkflowModeBypassCurrent,
				testCurrentMutation,
				&testNewSnapshot,
			)
			if currentExpectError || newExpectError {
				s.Error(err, err)
			} else {
				s.NoError(err, err)
			}
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) TestConflictResolveMode_UpdateCurrent() {

	// only reset workflow
	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	for state, expectError := range stateToError {
		testSnapshot := s.newTestWorkflowSnapshot(state)
		err := ValidateConflictResolveWorkflowModeState(
			ConflictResolveWorkflowModeUpdateCurrent,
			testSnapshot,
			nil,
			nil,
		)
		if !expectError {
			s.NoError(err, err)
		} else {
			s.Error(err, err)
		}
	}

	// reset workflow & new workflow
	resetStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	newStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	for resetState, resetExpectError := range resetStateToError {
		for newState, newExpectError := range newStateToError {
			testResetSnapshot := s.newTestWorkflowSnapshot(resetState)
			testNewSnapshot := s.newTestWorkflowSnapshot(newState)
			err := ValidateConflictResolveWorkflowModeState(
				ConflictResolveWorkflowModeUpdateCurrent,
				testResetSnapshot,
				&testNewSnapshot,
				nil,
			)
			if resetExpectError || newExpectError {
				s.Error(err, err)
			} else {
				s.NoError(err, err)
			}
		}
	}

	// reset workflow & current workflow
	resetStateToError = map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	currentStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for resetState, resetExpectError := range resetStateToError {
		for currentState, currentExpectError := range currentStateToError {
			testResetSnapshot := s.newTestWorkflowSnapshot(resetState)
			testCurrentSnapshot := s.newTestWorkflowMutation(currentState)
			err := ValidateConflictResolveWorkflowModeState(
				ConflictResolveWorkflowModeUpdateCurrent,
				testResetSnapshot,
				nil,
				&testCurrentSnapshot,
			)
			if resetExpectError || currentExpectError {
				s.Error(err, err)
			} else {
				s.NoError(err, err)
			}
		}
	}

	// reset workflow & new workflow & current workflow
	resetStateToError = map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	newStateToError = map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   false,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	currentStateToError = map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for resetState, resetExpectError := range resetStateToError {
		for newState, newExpectError := range newStateToError {
			for currentState, currentExpectError := range currentStateToError {
				testResetSnapshot := s.newTestWorkflowSnapshot(resetState)
				testNewSnapshot := s.newTestWorkflowSnapshot(newState)
				testCurrentSnapshot := s.newTestWorkflowMutation(currentState)
				err := ValidateConflictResolveWorkflowModeState(
					ConflictResolveWorkflowModeUpdateCurrent,
					testResetSnapshot,
					&testNewSnapshot,
					&testCurrentSnapshot,
				)
				if resetExpectError || newExpectError || currentExpectError {
					s.Error(err, err)
				} else {
					s.NoError(err, err)
				}
			}
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) TestConflictResolveMode_BypassCurrent() {

	// only reset workflow
	stateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for state, expectError := range stateToError {
		testSnapshot := s.newTestWorkflowSnapshot(state)
		err := ValidateConflictResolveWorkflowModeState(
			ConflictResolveWorkflowModeBypassCurrent,
			testSnapshot,
			nil,
			nil,
		)
		if !expectError {
			s.NoError(err, err)
		} else {
			s.Error(err, err)
		}
	}

	// reset workflow & new workflow
	resetStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: false,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    true,
	}
	newStateToError := map[enumsspb.WorkflowExecutionState]bool{
		enumsspb.WORKFLOW_EXECUTION_STATE_CREATED:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING:   true,
		enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED: true,
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE:    false,
	}
	for resetState, resetExpectError := range resetStateToError {
		for newState, newExpectError := range newStateToError {
			testResetSnapshot := s.newTestWorkflowSnapshot(resetState)
			testNewSnapshot := s.newTestWorkflowSnapshot(newState)
			err := ValidateConflictResolveWorkflowModeState(
				ConflictResolveWorkflowModeBypassCurrent,
				testResetSnapshot,
				&testNewSnapshot,
				nil,
			)
			if resetExpectError || newExpectError {
				if err == nil {
					fmt.Print("##")
				}
				s.Error(err, err)
			} else {
				s.NoError(err, err)
			}
		}
	}
}

func (s *validateOperationWorkflowModeStateSuite) newTestWorkflowSnapshot(
	state enumsspb.WorkflowExecutionState,
) WorkflowSnapshot {
	return WorkflowSnapshot{
		ExecutionInfo:  &persistencespb.WorkflowExecutionInfo{},
		ExecutionState: &persistencespb.WorkflowExecutionState{State: state},
	}
}

func (s *validateOperationWorkflowModeStateSuite) newTestWorkflowMutation(
	state enumsspb.WorkflowExecutionState,
) WorkflowMutation {
	return WorkflowMutation{
		ExecutionInfo:  &persistencespb.WorkflowExecutionInfo{},
		ExecutionState: &persistencespb.WorkflowExecutionState{State: state},
	}
}
