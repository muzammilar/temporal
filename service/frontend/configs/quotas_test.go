package configs

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/quotas"
	"go.temporal.io/server/common/testing/temporalapi"
)

var (
	testRateBurstFn        = quotas.NewDefaultIncomingRateBurst(func() float64 { return 5 })
	testOperatorRPSRatioFn = func() float64 { return 0.2 }
)

type (
	quotasSuite struct {
		suite.Suite
		*require.Assertions
	}
)

func TestQuotasSuite(t *testing.T) {
	s := new(quotasSuite)
	suite.Run(t, s)
}

func (s *quotasSuite) SetupSuite() {
}

func (s *quotasSuite) TearDownSuite() {
}

func (s *quotasSuite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *quotasSuite) TearDownTest() {
}

func (s *quotasSuite) TestExecutionAPIToPriorityMapping() {
	for _, priority := range APIToPriority {
		index := slices.Index(ExecutionAPIPrioritiesOrdered, priority)
		s.NotEqual(-1, index)
	}
}

func (s *quotasSuite) TestVisibilityAPIToPriorityMapping() {
	for _, priority := range VisibilityAPIToPriority {
		index := slices.Index(VisibilityAPIPrioritiesOrdered, priority)
		s.NotEqual(-1, index)
	}
}

func (s *quotasSuite) TestNamespaceReplicationInducingAPIToPriorityMapping() {
	for _, priority := range NamespaceReplicationInducingAPIToPriority {
		index := slices.Index(NamespaceReplicationInducingAPIPrioritiesOrdered, priority)
		s.NotEqual(-1, index)
	}
}

func (s *quotasSuite) TestExecutionAPIPrioritiesOrdered() {
	for idx := range ExecutionAPIPrioritiesOrdered[1:] {
		s.True(ExecutionAPIPrioritiesOrdered[idx] < ExecutionAPIPrioritiesOrdered[idx+1])
	}
}

func (s *quotasSuite) TestVisibilityAPIPrioritiesOrdered() {
	for idx := range VisibilityAPIPrioritiesOrdered[1:] {
		s.True(VisibilityAPIPrioritiesOrdered[idx] < VisibilityAPIPrioritiesOrdered[idx+1])
	}
}

func (s *quotasSuite) TestNamespaceReplicationInducingAPIPrioritiesOrdered() {
	for idx := range NamespaceReplicationInducingAPIPrioritiesOrdered[1:] {
		s.True(NamespaceReplicationInducingAPIPrioritiesOrdered[idx] < NamespaceReplicationInducingAPIPrioritiesOrdered[idx+1])
	}
}

func (s *quotasSuite) TestVisibilityAPIs() {
	apis := map[string]struct{}{
		"/temporal.api.workflowservice.v1.WorkflowService/GetWorkflowExecution":           {},
		"/temporal.api.workflowservice.v1.WorkflowService/CountWorkflowExecutions":        {},
		"/temporal.api.workflowservice.v1.WorkflowService/ScanWorkflowExecutions":         {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListOpenWorkflowExecutions":     {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListClosedWorkflowExecutions":   {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListWorkflowExecutions":         {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListArchivedWorkflowExecutions": {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListWorkers":                    {},
		"/temporal.api.workflowservice.v1.WorkflowService/DescribeWorker":                 {},

		"/temporal.api.workflowservice.v1.WorkflowService/GetWorkerTaskReachability":         {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListSchedules":                     {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListBatchOperations":               {},
		"/temporal.api.workflowservice.v1.WorkflowService/DescribeTaskQueueWithReachability": {},
		"/temporal.api.workflowservice.v1.WorkflowService/ListDeployments":                   {},
		"/temporal.api.workflowservice.v1.WorkflowService/GetDeploymentReachability":         {},
	}

	var service workflowservice.WorkflowServiceServer
	t := reflect.TypeOf(&service).Elem()
	apiToPriority := make(map[string]int, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		apiName := "/temporal.api.workflowservice.v1.WorkflowService/" + t.Method(i).Name
		if t.Method(i).Name == "DescribeTaskQueue" {
			apiName += "WithReachability"
		}
		if _, ok := apis[apiName]; ok {
			apiToPriority[apiName] = VisibilityAPIToPriority[apiName]
		}
	}
	s.Equal(apiToPriority, VisibilityAPIToPriority)
}

func (s *quotasSuite) TestNamespaceReplicationInducingAPIs() {
	apis := map[string]struct{}{
		"/temporal.api.workflowservice.v1.WorkflowService/RegisterNamespace":                {},
		"/temporal.api.workflowservice.v1.WorkflowService/UpdateNamespace":                  {},
		"/temporal.api.workflowservice.v1.WorkflowService/UpdateWorkerBuildIdCompatibility": {},
		"/temporal.api.workflowservice.v1.WorkflowService/UpdateWorkerVersioningRules":      {},
	}

	var service workflowservice.WorkflowServiceServer
	t := reflect.TypeOf(&service).Elem()
	apiToPriority := make(map[string]int, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		apiName := "/temporal.api.workflowservice.v1.WorkflowService/" + t.Method(i).Name
		if _, ok := apis[apiName]; ok {
			apiToPriority[apiName] = NamespaceReplicationInducingAPIToPriority[apiName]
		}
	}
	s.Equal(apiToPriority, NamespaceReplicationInducingAPIToPriority)
}

func (s *quotasSuite) TestAllAPIs() {
	apisWithPriority := make(map[string]struct{})
	for api := range APIToPriority {
		apisWithPriority[api] = struct{}{}
	}
	for api := range VisibilityAPIToPriority {
		apisWithPriority[api] = struct{}{}
	}
	for api := range NamespaceReplicationInducingAPIToPriority {
		apisWithPriority[api] = struct{}{}
	}
	var service workflowservice.WorkflowServiceServer
	temporalapi.WalkExportedMethods(&service, func(m reflect.Method) {
		_, ok := apisWithPriority["/temporal.api.workflowservice.v1.WorkflowService/"+m.Name]
		s.True(ok, "missing priority for API: %v", m.Name)
	})
	_, ok := apisWithPriority[DispatchNexusTaskByNamespaceAndTaskQueueAPIName]
	s.Truef(ok, "missing priority for API: %q", DispatchNexusTaskByNamespaceAndTaskQueueAPIName)
	_, ok = apisWithPriority[DispatchNexusTaskByEndpointAPIName]
	s.Truef(ok, "missing priority for API: %q", DispatchNexusTaskByEndpointAPIName)
	_, ok = apisWithPriority[CompleteNexusOperation]
	s.Truef(ok, "missing priority for API: %q", CompleteNexusOperation)
}

func (s *quotasSuite) TestOperatorPriority_Execution() {
	limiter := NewExecutionPriorityRateLimiter(testRateBurstFn, testOperatorRPSRatioFn)
	s.testOperatorPrioritized(limiter, "DescribeWorkflowExecution")
}

func (s *quotasSuite) TestOperatorPriority_Visibility() {
	limiter := NewVisibilityPriorityRateLimiter(testRateBurstFn, testOperatorRPSRatioFn)
	s.testOperatorPrioritized(limiter, "ListOpenWorkflowExecutions")
}

func (s *quotasSuite) TestOperatorPriority_NamespaceReplicationInducing() {
	limiter := NewNamespaceReplicationInducingAPIPriorityRateLimiter(testRateBurstFn, testOperatorRPSRatioFn)
	s.testOperatorPrioritized(limiter, "RegisterNamespace")
}

func (s *quotasSuite) testOperatorPrioritized(limiter quotas.RequestRateLimiter, api string) {
	operatorRequest := quotas.NewRequest(
		api,
		1,
		"test-namespace",
		headers.CallerTypeOperator,
		-1,
		"")

	apiRequest := quotas.NewRequest(
		api,
		1,
		"test-namespace",
		headers.CallerTypeAPI,
		-1,
		"")

	requestTime := time.Now()
	limitCount := 0

	for i := 0; i < 12; i++ {
		if !limiter.Allow(requestTime, apiRequest) {
			limitCount++
			s.True(limiter.Allow(requestTime, operatorRequest))
		}
	}
	s.Equal(2, limitCount)
}
