package gcloud

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	archiverspb "go.temporal.io/server/api/archiver/v1"
	"go.temporal.io/server/common/archiver"
	"go.temporal.io/server/common/archiver/gcloud/connector"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/primitives/timestamp"
	"go.temporal.io/server/common/searchattribute"
	"go.temporal.io/server/common/testing/protorequire"
	"go.temporal.io/server/common/util"
	"go.uber.org/mock/gomock"
)

const (
	testWorkflowTypeName     = "test-workflow-type"
	exampleVisibilityRecord  = `{"namespaceId":"test-namespace-id","namespace":"test-namespace","workflowId":"test-workflow-id","runId":"test-run-id","workflowTypeName":"test-workflow-type","startTime":"2020-02-05T09:56:14.804475Z","closeTime":"2020-02-05T09:56:15.946478Z","status":"Completed","historyLength":36,"memo":null,"searchAttributes":null,"historyArchivalUri":"gs://my-bucket-cad/temporal_archival/development"}`
	exampleVisibilityRecord2 = `{"namespaceId":"test-namespace-id","namespace":"test-namespace",
"workflowId":"test-workflow-id2","runId":"test-run-id","workflowTypeName":"test-workflow-type",
"startTime":"2020-02-05T09:56:14.804475Z","closeTime":"2020-02-05T09:56:15.946478Z","status":"Completed","historyLength":36,"memo":null,"searchAttributes":null,"historyArchivalUri":"gs://my-bucket-cad/temporal_archival/development"}`
)

func (s *visibilityArchiverSuite) SetupTest() {
	s.Assertions = require.New(s.T())
	s.controller = gomock.NewController(s.T())
	s.logger = log.NewNoopLogger()
	s.metricsHandler = metrics.NoopMetricsHandler
	s.expectedVisibilityRecords = []*archiverspb.VisibilityRecord{
		{
			NamespaceId:      testNamespaceID,
			Namespace:        testNamespace,
			WorkflowId:       testWorkflowID,
			RunId:            testRunID,
			WorkflowTypeName: testWorkflowTypeName,
			StartTime:        timestamp.UnixOrZeroTimePtr(1580896574804475000),
			CloseTime:        timestamp.UnixOrZeroTimePtr(1580896575946478000),
			Status:           enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
			HistoryLength:    36,
		},
	}
}

func (s *visibilityArchiverSuite) TearDownTest() {
	s.controller.Finish()
}

func TestVisibilityArchiverSuiteSuite(t *testing.T) {
	suite.Run(t, new(visibilityArchiverSuite))
}

type visibilityArchiverSuite struct {
	*require.Assertions
	protorequire.ProtoAssertions
	suite.Suite
	controller                *gomock.Controller
	logger                    log.Logger
	metricsHandler            metrics.Handler
	expectedVisibilityRecords []*archiverspb.VisibilityRecord
}

func (s *visibilityArchiverSuite) TestValidateVisibilityURI() {
	testCases := []struct {
		URI         string
		expectedErr error
	}{
		{
			URI:         "wrongscheme:///a/b/c",
			expectedErr: archiver.ErrURISchemeMismatch,
		},
		{
			URI:         "gs:my-bucket-cad/temporal_archival/visibility",
			expectedErr: archiver.ErrInvalidURI,
		},
		{
			URI:         "gs://",
			expectedErr: archiver.ErrInvalidURI,
		},
		{
			URI:         "gs://my-bucket-cad",
			expectedErr: archiver.ErrInvalidURI,
		},
		{
			URI:         "gs:/my-bucket-cad/temporal_archival/visibility",
			expectedErr: archiver.ErrInvalidURI,
		},
		{
			URI:         "gs://my-bucket-cad/temporal_archival/visibility",
			expectedErr: nil,
		},
	}

	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), gomock.Any(), "").Return(false, nil)
	visibilityArchiver := new(visibilityArchiver)
	visibilityArchiver.gcloudStorage = storageWrapper
	for _, tc := range testCases {
		URI, err := archiver.NewURI(tc.URI)
		s.NoError(err)
		s.Equal(tc.expectedErr, visibilityArchiver.ValidateURI(URI))
	}
}

func (s *visibilityArchiverSuite) TestArchive_Fail_InvalidVisibilityURI() {
	ctx := context.Background()
	URI, err := archiver.NewURI("wrongscheme://")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)

	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)
	request := &archiverspb.VisibilityRecord{
		NamespaceId: testNamespaceID,
		Namespace:   testNamespace,
		WorkflowId:  testWorkflowID,
		RunId:       testRunID,
	}

	err = visibilityArchiver.Archive(ctx, URI, request)
	s.Error(err)
}

func (s *visibilityArchiverSuite) TestQuery_Fail_InvalidVisibilityURI() {
	ctx := context.Background()
	URI, err := archiver.NewURI("wrongscheme://")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)

	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)
	request := &archiver.QueryVisibilityRequest{
		NamespaceID: testNamespaceID,
		PageSize:    10,
		Query:       "WorkflowType='type::example' AND CloseTime='2020-02-05T11:00:00Z' AND SearchPrecision='Day'",
	}

	_, err = visibilityArchiver.Query(ctx, URI, request, searchattribute.TestNameTypeMap)
	s.Error(err)
}

func (s *visibilityArchiverSuite) TestVisibilityArchive() {
	ctx := context.Background()
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	storageWrapper.EXPECT().Upload(gomock.Any(), URI, gomock.Any(), gomock.Any()).Return(nil).Times(2)

	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)

	request := &archiverspb.VisibilityRecord{
		Namespace:        testNamespace,
		NamespaceId:      testNamespaceID,
		WorkflowId:       testWorkflowID,
		RunId:            testRunID,
		WorkflowTypeName: testWorkflowTypeName,
		StartTime:        timestamp.TimeNowPtrUtc(),
		ExecutionTime:    nil, // workflow without backoff
		CloseTime:        timestamp.TimeNowPtrUtc(),
		Status:           enumspb.WORKFLOW_EXECUTION_STATUS_FAILED,
		HistoryLength:    int64(101),
	}

	err = visibilityArchiver.Archive(ctx, URI, request)
	s.NoError(err)
}

func (s *visibilityArchiverSuite) TestQuery_Fail_InvalidQuery() {
	ctx := context.Background()
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)

	mockParser := NewMockQueryParser(s.controller)
	mockParser.EXPECT().Parse(gomock.Any()).Return(nil, errors.New("invalid query"))
	visibilityArchiver.queryParser = mockParser
	response, err := visibilityArchiver.Query(ctx, URI, &archiver.QueryVisibilityRequest{
		NamespaceID: "some random namespaceID",
		PageSize:    10,
		Query:       "some invalid query",
	}, searchattribute.TestNameTypeMap)
	s.Error(err)
	s.Nil(response)
}

func (s *visibilityArchiverSuite) TestQuery_Fail_InvalidToken() {
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)

	mockParser := NewMockQueryParser(s.controller)
	startTime, _ := time.Parse(time.RFC3339, "2019-10-04T11:00:00+00:00")
	closeTime := startTime.Add(time.Hour)
	precision := PrecisionDay
	mockParser.EXPECT().Parse(gomock.Any()).Return(&parsedQuery{
		closeTime:       closeTime,
		startTime:       startTime,
		searchPrecision: &precision,
	}, nil)
	visibilityArchiver.queryParser = mockParser
	request := &archiver.QueryVisibilityRequest{
		NamespaceID:   testNamespaceID,
		Query:         "parsed by mockParser",
		PageSize:      1,
		NextPageToken: []byte{1, 2, 3},
	}
	response, err := visibilityArchiver.Query(context.Background(), URI, request, searchattribute.TestNameTypeMap)
	s.Error(err)
	s.Nil(response)
}

func (s *visibilityArchiverSuite) TestQuery_Success_NoNextPageToken() {
	ctx := context.Background()
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	storageWrapper.EXPECT().QueryWithFilters(gomock.Any(), URI, gomock.Any(), 10, 0, gomock.Any()).Return([]string{"closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility"}, true, 1, nil)
	storageWrapper.EXPECT().Get(gomock.Any(), URI, "test-namespace-id/closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility").Return([]byte(exampleVisibilityRecord), nil)

	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)

	mockParser := NewMockQueryParser(s.controller)
	dayPrecision := "Day"
	closeTime, _ := time.Parse(time.RFC3339, "2019-10-04T11:00:00+00:00")
	mockParser.EXPECT().Parse(gomock.Any()).Return(&parsedQuery{
		closeTime:       closeTime,
		searchPrecision: &dayPrecision,
		workflowType:    util.Ptr("MobileOnlyWorkflow::processMobileOnly"),
		workflowID:      util.Ptr(testWorkflowID),
		runID:           util.Ptr(testRunID),
	}, nil)
	visibilityArchiver.queryParser = mockParser
	request := &archiver.QueryVisibilityRequest{
		NamespaceID: testNamespaceID,
		PageSize:    10,
		Query:       "parsed by mockParser",
	}

	response, err := visibilityArchiver.Query(ctx, URI, request, searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.NotNil(response)
	s.Nil(response.NextPageToken)
	s.Len(response.Executions, 1)
	ei, err := convertToExecutionInfo(s.expectedVisibilityRecords[0], searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.ProtoEqual(ei, response.Executions[0])
}

func (s *visibilityArchiverSuite) TestQuery_Success_SmallPageSize() {
	pageSize := 2
	ctx := context.Background()
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil).Times(2)
	storageWrapper.EXPECT().QueryWithFilters(gomock.Any(), URI, gomock.Any(), pageSize, 0, gomock.Any()).Return([]string{"closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility", "closeTimeout_2020-02-05T09:56:15Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility"}, false, 1, nil)
	storageWrapper.EXPECT().QueryWithFilters(gomock.Any(), URI, gomock.Any(), pageSize, 1, gomock.Any()).Return([]string{"closeTimeout_2020-02-05T09:56:16Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility"}, true, 2, nil)
	storageWrapper.EXPECT().Get(gomock.Any(), URI, "test-namespace-id/closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility").Return([]byte(exampleVisibilityRecord), nil)
	storageWrapper.EXPECT().Get(gomock.Any(), URI, "test-namespace-id/closeTimeout_2020-02-05T09:56:15Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility").Return([]byte(exampleVisibilityRecord), nil)
	storageWrapper.EXPECT().Get(gomock.Any(), URI, "test-namespace-id/closeTimeout_2020-02-05T09:56:16Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility").Return([]byte(exampleVisibilityRecord), nil)

	visibilityArchiver := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	s.NoError(err)

	mockParser := NewMockQueryParser(s.controller)
	dayPrecision := "Day"
	closeTime, _ := time.Parse(time.RFC3339, "2019-10-04T11:00:00+00:00")
	mockParser.EXPECT().Parse(gomock.Any()).Return(&parsedQuery{
		closeTime:       closeTime,
		searchPrecision: &dayPrecision,
		workflowType:    util.Ptr("MobileOnlyWorkflow::processMobileOnly"),
		workflowID:      util.Ptr(testWorkflowID),
		runID:           util.Ptr(testRunID),
	}, nil).AnyTimes()
	visibilityArchiver.queryParser = mockParser
	request := &archiver.QueryVisibilityRequest{
		NamespaceID: testNamespaceID,
		PageSize:    pageSize,
		Query:       "parsed by mockParser",
	}

	response, err := visibilityArchiver.Query(ctx, URI, request, searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.NotNil(response)
	s.NotNil(response.NextPageToken)
	s.Len(response.Executions, 2)
	ei, err := convertToExecutionInfo(s.expectedVisibilityRecords[0], searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.ProtoEqual(ei, response.Executions[0])
	ei, err = convertToExecutionInfo(s.expectedVisibilityRecords[0], searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.ProtoEqual(ei, response.Executions[1])

	request.NextPageToken = response.NextPageToken
	response, err = visibilityArchiver.Query(ctx, URI, request, searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.NotNil(response)
	s.Nil(response.NextPageToken)
	s.Len(response.Executions, 1)
	ei, err = convertToExecutionInfo(s.expectedVisibilityRecords[0], searchattribute.TestNameTypeMap)
	s.NoError(err)
	s.ProtoEqual(ei, response.Executions[0])
}

func (s *visibilityArchiverSuite) TestQuery_EmptyQuery_InvalidNamespace() {
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	arc := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)
	req := &archiver.QueryVisibilityRequest{
		NamespaceID:   "",
		PageSize:      1,
		NextPageToken: nil,
		Query:         "",
	}
	_, err = arc.Query(context.Background(), URI, req, searchattribute.TestNameTypeMap)

	var svcErr *serviceerror.InvalidArgument

	s.ErrorAs(err, &svcErr)
}

func (s *visibilityArchiverSuite) TestQuery_EmptyQuery_ZeroPageSize() {
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(false, nil)
	arc := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)

	req := &archiver.QueryVisibilityRequest{
		NamespaceID:   testNamespaceID,
		PageSize:      0,
		NextPageToken: nil,
		Query:         "",
	}
	_, err = arc.Query(context.Background(), URI, req, searchattribute.TestNameTypeMap)

	var svcErr *serviceerror.InvalidArgument

	s.ErrorAs(err, &svcErr)
}

func (s *visibilityArchiverSuite) TestQuery_EmptyQuery_Pagination() {
	URI, err := archiver.NewURI("gs://my-bucket-cad/temporal_archival/visibility")
	s.NoError(err)
	storageWrapper := connector.NewMockClient(s.controller)
	storageWrapper.EXPECT().Exist(gomock.Any(), URI, gomock.Any()).Return(true, nil).Times(2)
	storageWrapper.EXPECT().QueryWithFilters(
		gomock.Any(),
		URI,
		gomock.Any(),
		1,
		0,
		gomock.Any(),
	).Return(
		[]string{"closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility"},
		false,
		1,
		nil,
	)
	storageWrapper.EXPECT().QueryWithFilters(
		gomock.Any(),
		URI,
		gomock.Any(),
		1,
		1,
		gomock.Any(),
	).Return(
		[]string{"closeTimeout_2020-02-05T09:56:14Z_test-workflow-id2_MobileOnlyWorkflow::processMobileOnly_test-run" +
			"-id.visibility"},
		true,
		2,
		nil,
	)
	storageWrapper.EXPECT().Get(
		gomock.Any(),
		URI,
		"test-namespace-id/closeTimeout_2020-02-05T09:56:14Z_test-workflow-id_MobileOnlyWorkflow::processMobileOnly_test-run-id.visibility",
	).Return([]byte(exampleVisibilityRecord), nil)
	storageWrapper.EXPECT().Get(gomock.Any(), URI,
		"test-namespace-id/closeTimeout_2020-02-05T09:56:14Z_test-workflow-id2_MobileOnlyWorkflow"+
			"::processMobileOnly_test-run-id.visibility").Return([]byte(exampleVisibilityRecord2), nil)

	arc := newVisibilityArchiver(s.logger, s.metricsHandler, storageWrapper)

	response := &archiver.QueryVisibilityResponse{
		Executions:    nil,
		NextPageToken: nil,
	}

	limit := 10
	executions := make(map[string]*workflowpb.WorkflowExecutionInfo, limit)

	numPages := 2
	for i := 0; i < numPages; i++ {
		req := &archiver.QueryVisibilityRequest{
			NamespaceID:   testNamespaceID,
			PageSize:      1,
			NextPageToken: response.NextPageToken,
			Query:         "",
		}
		response, err = arc.Query(context.Background(), URI, req, searchattribute.TestNameTypeMap)
		s.NoError(err)
		s.NotNil(response)
		s.Len(response.Executions, 1)

		s.Equal(
			i == numPages-1,
			response.NextPageToken == nil,
			"should have no next page token on the last iteration",
		)

		for _, execution := range response.Executions {
			key := execution.Execution.GetWorkflowId() +
				"/" + execution.Execution.GetRunId() +
				"/" + execution.CloseTime.String()
			executions[key] = execution
		}
	}
	s.Len(executions, 2, "there should be exactly 2 unique executions")
}
