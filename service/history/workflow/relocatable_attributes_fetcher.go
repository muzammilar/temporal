package workflow

import (
	"context"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/persistence/visibility/manager"
	"go.temporal.io/server/service/history/configs"
	historyi "go.temporal.io/server/service/history/interfaces"
)

// RelocatableAttributesFetcher is used to fetch the relocatable attributes from the mutable state.
// Relocatable attributes are attributes that can be moved from the mutable state to the persistence backend.
type RelocatableAttributesFetcher interface {
	Fetch(
		ctx context.Context,
		mutableState historyi.MutableState,
	) (*RelocatableAttributes, error)
}

// RelocatableAttributesFetcherProvider provides a new instance of a RelocatableAttributesFetcher.
// The manager.VisibilityManager parameter is used to fetch the relocatable attributes from the persistence backend iff
// we already moved them there out from the mutable state.
// The visibility manager is not used if the relocatable attributes are still in the mutable state.
// We detect that the fields have moved by checking the RelocatableAttributesRemoved flag in the mutable state.
// Because the relocatable fields that we push to persistence are never updated thereafter,
// we may cache them on a per-workflow execution basis.
// Currently, there is no cache, but you may provide a manager.VisibilityManager that supports caching to this function
// safely.
// TODO: Add a cache around the visibility manager for the relocatable attributes.
func RelocatableAttributesFetcherProvider(
	config *configs.Config,
	visibilityManager manager.VisibilityManager,
) RelocatableAttributesFetcher {
	return &relocatableAttributesFetcher{
		visibilityManager:          visibilityManager,
		disableFetchFromVisibility: config.DisableFetchRelocatableAttributesFromVisibility,
	}
}

// RelocatableAttributes contains workflow attributes that can be moved from the mutable state to the persistence
// backend.
type RelocatableAttributes struct {
	Memo             *commonpb.Memo
	SearchAttributes *commonpb.SearchAttributes
}

// relocatableAttributesFetcher is the default implementation of RelocatableAttributesFetcher.
type relocatableAttributesFetcher struct {
	visibilityManager manager.VisibilityManager

	disableFetchFromVisibility dynamicconfig.BoolPropertyFnWithNamespaceFilter
}

// Fetch fetches the relocatable attributes from the mutable state or the persistence backend.
// First, it checks if the close visibility task clean up was executed. If it was, then the relocatable attributes
// are fetched from the persistence backend. Otherwise, the relocatable attributes are fetched from the mutable state.
func (f *relocatableAttributesFetcher) Fetch(
	ctx context.Context,
	mutableState historyi.MutableState,
) (*RelocatableAttributes, error) {
	executionInfo := mutableState.GetExecutionInfo()
	// If the relocatable attributes were not removed from mutable state, then we can fetch the memo
	// and search attributes from the mutable state.
	if !executionInfo.GetRelocatableAttributesRemoved() {
		return &RelocatableAttributes{
			Memo:             &commonpb.Memo{Fields: executionInfo.Memo},
			SearchAttributes: &commonpb.SearchAttributes{IndexedFields: executionInfo.SearchAttributes},
		}, nil
	}

	if f.disableFetchFromVisibility(mutableState.GetNamespaceEntry().Name().String()) {
		return &RelocatableAttributes{}, nil
	}

	// If we have processed close visibility task, then we need to fetch the search attributes and memo from the
	// persistence backend because we have already deleted them from the mutable state.
	executionState := mutableState.GetExecutionState()
	visResponse, err := f.visibilityManager.GetWorkflowExecution(
		ctx,
		&manager.GetWorkflowExecutionRequest{
			NamespaceID: mutableState.GetNamespaceEntry().ID(),
			Namespace:   mutableState.GetNamespaceEntry().Name(),
			RunID:       executionState.GetRunId(),
			WorkflowID:  executionInfo.GetWorkflowId(),
		},
	)
	if err != nil {
		return nil, err
	}
	return &RelocatableAttributes{
		Memo:             visResponse.Execution.Memo,
		SearchAttributes: visResponse.Execution.SearchAttributes,
	}, nil
}
