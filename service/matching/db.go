package matching

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/persistence"
	"go.temporal.io/server/common/primitives/timestamp"
	"go.temporal.io/server/common/softassert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	initialRangeID     = 1 // Id of the first range of a new task queue
	stickyTaskQueueTTL = 24 * time.Hour

	// Subqueue zero corresponds to "the queue" before migrating metadata to subqueues.
	// For backwards compatibility, some operations only apply to subqueue zero for now.
	subqueueZero = 0
)

type (
	taskQueueDB struct {
		sync.Mutex
		config         *taskQueueConfig
		queue          *PhysicalTaskQueueKey
		rangeID        int64
		store          persistence.TaskManager
		logger         log.Logger
		metricsHandler metrics.Handler
		subqueues      []*dbSubqueue

		// used to avoid unnecessary metadata writes:
		lastChange time.Time // updated when metadata is changed in memory
		lastWrite  time.Time // updated when metadata is successfully written to db
	}

	dbSubqueue struct {
		persistencespb.SubqueueInfo
		maxReadLevel int64
		oldestTime   time.Time // time of oldest task if backlog, otherwise zero time
	}

	taskQueueState struct {
		rangeID   int64
		ackLevel  int64 // TODO(pri): old matcher cleanup, delete later
		subqueues []persistencespb.SubqueueInfo
	}

	createTasksResponse struct {
		bySubqueue map[int]subqueueCreateTasksResponse
	}

	subqueueCreateTasksResponse struct {
		tasks              []*persistencespb.AllocatedTaskInfo
		maxReadLevelBefore int64
		maxReadLevelAfter  int64
	}

	createFairTasksResponse map[int][]*persistencespb.AllocatedTaskInfo // subqueue -> tasks
)

// newTaskQueueDB returns an instance of an object that represents
// persistence view of a physical task queue. All mutations / reads to queues
// wrt persistence go through this object.
//
// This class will serialize writes to persistence that do condition updates. There are
// two reasons for doing this:
//   - To work around known Cassandra issue where concurrent LWT to the same partition cause timeout errors
//   - To provide the guarantee that there is only writer who updates queue in persistence at any given point in time
//     This guarantee makes some of the other code simpler and there is no impact to perf because updates to taskqueue are
//     spread out and happen in background routines
func newTaskQueueDB(
	config *taskQueueConfig,
	store persistence.TaskManager,
	queue *PhysicalTaskQueueKey,
	logger log.Logger,
	metricsHandler metrics.Handler,
) *taskQueueDB {
	return &taskQueueDB{
		config:         config,
		queue:          queue,
		store:          store,
		logger:         logger,
		metricsHandler: metricsHandler,
	}
}

// RangeID returns the current persistence view of rangeID
func (db *taskQueueDB) RangeID() int64 {
	db.Lock()
	defer db.Unlock()
	return db.rangeID
}

// GetMaxReadLevel returns the current maxReadLevel
func (db *taskQueueDB) GetMaxReadLevel(subqueue int) int64 {
	db.Lock()
	defer db.Unlock()
	return db.getMaxReadLevelLocked(subqueue)
}

func (db *taskQueueDB) getMaxReadLevelLocked(subqueue int) int64 {
	return db.subqueues[subqueue].maxReadLevel
}

// GetMaxReadLevel returns the current maxReadLevel
func (db *taskQueueDB) GetMaxFairReadLevel(subqueue int) fairLevel {
	db.Lock()
	defer db.Unlock()
	return db.getMaxFairReadLevelLocked(subqueue)
}

func (db *taskQueueDB) getMaxFairReadLevelLocked(subqueue int) fairLevel {
	return fairLevelFromProto(db.subqueues[subqueue].FairMaxReadLevel)
}

// This is only exposed for testing!
func (db *taskQueueDB) setMaxReadLevelForTesting(subqueue int, level int64) {
	db.Lock()
	defer db.Unlock()
	db.subqueues[subqueue].maxReadLevel = level
}

// RenewLease renews the lease on a taskqueue. If there is no previous lease,
// this method will attempt to steal taskqueue from current owner
func (db *taskQueueDB) RenewLease(
	ctx context.Context,
) (taskQueueState, error) {
	db.Lock()
	defer db.Unlock()

	if db.rangeID == 0 {
		if err := db.takeOverTaskQueueLocked(ctx); err != nil {
			return taskQueueState{}, err
		}
	} else {
		if err := db.updateTaskQueueLocked(ctx, true); err != nil {
			return taskQueueState{}, err
		}
	}
	return taskQueueState{
		rangeID:   db.rangeID,
		ackLevel:  db.subqueues[subqueueZero].AckLevel, // TODO(pri): cleanup, only used by old backlog manager
		subqueues: db.cloneSubqueues(),
	}, nil
}

func (db *taskQueueDB) takeOverTaskQueueLocked(
	ctx context.Context,
) error {
	response, err := db.store.GetTaskQueue(ctx, &persistence.GetTaskQueueRequest{
		NamespaceID: db.queue.NamespaceId(),
		TaskQueue:   db.queue.PersistenceName(),
		TaskType:    db.queue.TaskType(),
	})
	switch err.(type) {
	case nil:
		db.rangeID = response.RangeID
		db.subqueues = db.ensureDefaultSubqueuesLocked(
			response.TaskQueueInfo.Subqueues,
			response.TaskQueueInfo.AckLevel,
			response.TaskQueueInfo.ApproximateBacklogCount,
		)
		err := db.updateTaskQueueLocked(ctx, true)
		if err != nil {
			db.rangeID = 0
			return err
		}
		// We took over the task queue and are not sure what tasks may have been written
		// before. Set max read level id of all subqueues to just before our new block.
		maxReadLevel := rangeIDToTaskIDBlock(db.rangeID, db.config.RangeSize).start - 1
		for _, s := range db.subqueues {
			s.maxReadLevel = maxReadLevel
		}
		return nil

	case *serviceerror.NotFound:
		db.rangeID = initialRangeID
		db.subqueues = db.ensureDefaultSubqueuesLocked(nil, 0, 0)
		if _, err := db.store.CreateTaskQueue(ctx, &persistence.CreateTaskQueueRequest{
			RangeID:       db.rangeID,
			TaskQueueInfo: db.cachedQueueInfo(),
		}); err != nil {
			db.rangeID = 0
			return err
		}
		db.lastWrite = time.Now()
		// In this case, ensureDefaultSubqueuesLocked already initialized subqueue 0 to have
		// ackLevel and maxReadLevel 0, so we don't need to initialize them.
		softassert.That(db.logger, db.subqueues[0].maxReadLevel == 0, "should have maxReadLevel 0 here")
		softassert.That(db.logger, db.subqueues[0].FairMaxReadLevel == nil, "should have maxReadLevel 0 here")
		softassert.That(db.logger, db.subqueues[0].AckLevel == 0, "should have ackLevel 0 here")
		softassert.That(db.logger, db.subqueues[0].FairAckLevel == nil, "should have ackLevel 0 here")
		return nil

	default:
		return err
	}
}

func (db *taskQueueDB) updateTaskQueueLocked(ctx context.Context, incrementRangeId bool) error {
	newRangeID := db.rangeID
	if incrementRangeId {
		newRangeID++
	}
	if _, err := db.store.UpdateTaskQueue(ctx, &persistence.UpdateTaskQueueRequest{
		RangeID:       newRangeID,
		TaskQueueInfo: db.cachedQueueInfo(),
		PrevRangeID:   db.rangeID,
	}); err != nil {
		return err
	}
	db.lastWrite = time.Now()
	db.rangeID = newRangeID
	return nil
}

// OldUpdateState updates the queue state with the given value. This is used by old backlog
// manager (not subqueue-enabled).
// TODO(pri): old matcher cleanup
func (db *taskQueueDB) OldUpdateState(
	ctx context.Context,
	ackLevel int64,
) error {
	db.Lock()
	defer db.Unlock()
	// We don't need to update lastWrite/lastChange in here since this function is only used by
	// the old backlog manager and those fields are only used by the new backlog manager.

	// Reset approximateBacklogCount to fix the count divergence issue
	maxReadLevel := db.getMaxReadLevelLocked(subqueueZero)
	if ackLevel == maxReadLevel {
		db.subqueues[subqueueZero].ApproximateBacklogCount = 0
		db.subqueues[subqueueZero].oldestTime = time.Time{} // zero time means no backlog
	}

	prevAckLevel := db.subqueues[subqueueZero].AckLevel
	db.subqueues[subqueueZero].AckLevel = ackLevel

	err := db.updateTaskQueueLocked(ctx, false)
	if err != nil {
		db.subqueues[subqueueZero].AckLevel = prevAckLevel
	}
	db.emitBacklogGaugesLocked()
	return err
}

func (db *taskQueueDB) SyncState(ctx context.Context) error {
	db.Lock()
	defer db.Unlock()
	defer db.emitBacklogGaugesLocked()

	// We only need to write if something changed, or if we're past half of the sticky queue TTL.
	// Note that we use the same threshold for non-sticky queues even though they don't have a
	// persistence TTL, since the scavenger looks for metadata that hasn't been updated in 48 hours.
	needWrite := db.lastChange.After(db.lastWrite) || time.Since(db.lastWrite) > stickyTaskQueueTTL/2
	if !needWrite {
		return nil
	}

	return db.updateTaskQueueLocked(ctx, false)
}

func (db *taskQueueDB) updateAckLevelAndBacklogStats(subqueue int, newAckLevel int64, countDelta int64, oldestTime time.Time) {
	db.Lock()
	defer db.Unlock()

	dbQueue := db.subqueues[subqueue]
	if newAckLevel < dbQueue.AckLevel {
		softassert.Fail(db.logger,
			fmt.Sprintf("ack level in subqueue %d should not move backwards (from %v to %v)",
				subqueue, dbQueue.AckLevel, newAckLevel))
	}
	if dbQueue.AckLevel != newAckLevel {
		db.lastChange = time.Now()
		dbQueue.AckLevel = newAckLevel
	}

	if newAckLevel == db.getMaxReadLevelLocked(subqueue) {
		// Reset approximateBacklogCount to fix the count divergence issue
		if dbQueue.ApproximateBacklogCount != 0 || !dbQueue.oldestTime.Equal(oldestTime) {
			db.lastChange = time.Now()
			dbQueue.ApproximateBacklogCount = 0
			dbQueue.oldestTime = oldestTime
		}
	} else if countDelta != 0 {
		db.lastChange = time.Now()
		db.updateBacklogStatsLocked(subqueue, countDelta, oldestTime)
	}
}

func (db *taskQueueDB) updateFairAckLevel(subqueue int, newAckLevel fairLevel, countDelta, knownCount int64, oldestTime time.Time) {
	db.Lock()
	defer db.Unlock()

	db.lastChange = time.Now()
	dbQueue := db.subqueues[subqueue]
	if prev := fairLevelFromProto(dbQueue.FairAckLevel); newAckLevel.less(prev) {
		softassert.Fail(db.logger,
			fmt.Sprintf("ack level in subqueue %d should not move backwards (from %v to %v)",
				subqueue, prev, newAckLevel))
	}
	dbQueue.FairAckLevel = newAckLevel.toProto()

	if knownCount >= 0 {
		// Reset approximateBacklogCount to fix the count divergence issue
		dbQueue.ApproximateBacklogCount = knownCount
		dbQueue.oldestTime = oldestTime
	} else if countDelta != 0 {
		db.updateBacklogStatsLocked(subqueue, countDelta, oldestTime)
	}
}

// Use this to reset ApproximateBacklogCount when the backlog count is known, e.g. when you're
// read to the end of the backlog.
func (db *taskQueueDB) setKnownFairBacklogCount(subqueue int, count int64) {
	db.Lock()
	defer db.Unlock()

	if db.subqueues[subqueue].ApproximateBacklogCount != count {
		db.lastChange = time.Now()
		db.subqueues[subqueue].ApproximateBacklogCount = count
		if count == 0 {
			db.subqueues[subqueue].oldestTime = time.Time{}
		}
	}
}

// updateApproximateBacklogCount updates the in-memory DB state with the given delta value
// TODO(pri): old matcher cleanup
func (db *taskQueueDB) updateBacklogStats(countDelta int64, oldestTime time.Time) {
	db.Lock()
	defer db.Unlock()
	db.lastChange = time.Now()
	db.updateBacklogStatsLocked(0, countDelta, oldestTime)
}

func (db *taskQueueDB) updateBacklogStatsLocked(subqueue int, countDelta int64, oldestTime time.Time) {
	// Prevent under-counting
	count := &db.subqueues[subqueue].ApproximateBacklogCount
	if *count+countDelta < 0 {
		db.logger.Info("ApproximateBacklogCount could have under-counted.",
			tag.WorkerBuildId(db.queue.Version().MetricsTagValue()),
			tag.WorkflowNamespaceID(db.queue.Partition().NamespaceId()))
		*count = 0
	} else {
		*count += countDelta
	}
	db.subqueues[subqueue].oldestTime = oldestTime
}

// getApproximateBacklogCountsBySubqueue return the approximate backlog count for each subqueue.
// The index corresponds to the subqueue id.
func (db *taskQueueDB) getApproximateBacklogCountsBySubqueue() []int64 {
	db.Lock()
	defer db.Unlock()

	result := make([]int64, len(db.subqueues))
	for id, s := range db.subqueues {
		result[id] = s.ApproximateBacklogCount
	}
	return result
}

func (db *taskQueueDB) getApproximateBacklogCountAndMaxReadLevel(subqueue int) (int64, fairLevel) {
	db.Lock()
	defer db.Unlock()
	s := db.subqueues[subqueue]
	return s.ApproximateBacklogCount, fairLevelFromProto(s.FairMaxReadLevel)
}

func (db *taskQueueDB) getTotalApproximateBacklogCount() int64 {
	db.Lock()
	defer db.Unlock()

	var total int64
	for _, s := range db.subqueues {
		total += s.ApproximateBacklogCount
	}
	return total
}

// CreateTasks creates a batch of given tasks for this task queue
func (db *taskQueueDB) CreateTasks(
	ctx context.Context,
	reqs []*writeTaskRequest,
) (createTasksResponse, error) {
	db.Lock()
	defer db.Unlock()

	if len(reqs) == 0 {
		return createTasksResponse{}, nil
	}

	updates := make(map[int]subqueueCreateTasksResponse)
	allTasks := make([]*persistencespb.AllocatedTaskInfo, len(reqs))
	allSubqueues := make([]int, len(reqs))
	for i, req := range reqs {
		task := &persistencespb.AllocatedTaskInfo{
			TaskId: req.id,
			Data:   req.taskInfo,
		}
		allTasks[i] = task
		allSubqueues[i] = req.subqueue

		u := updates[req.subqueue]
		updates[req.subqueue] = subqueueCreateTasksResponse{
			tasks:              append(u.tasks, task),
			maxReadLevelBefore: db.getMaxReadLevelLocked(req.subqueue),
			maxReadLevelAfter:  task.TaskId, // task ids are in order so this is the max
		}
	}

	for sq, update := range updates {
		db.subqueues[sq].ApproximateBacklogCount += int64(len(update.tasks))
	}

	resp, err := db.store.CreateTasks(
		ctx,
		&persistence.CreateTasksRequest{
			TaskQueueInfo: &persistence.PersistedTaskQueueInfo{
				Data:    db.cachedQueueInfo(),
				RangeID: db.rangeID,
			},
			Tasks:     allTasks,
			Subqueues: allSubqueues,
		})

	// Update the maxReadLevel after the writes are completed, but before we send the response,
	// so that taskReader is guaranteed to see the new read level when SpoolTask wakes it up.
	// Do this even if the write fails, we won't reuse the task ids.
	for sq, update := range updates {
		db.subqueues[sq].maxReadLevel = update.maxReadLevelAfter
	}

	if err == nil {
		// Only update lastWrite for persistence implementations that update metadata on CreateTasks.
		if resp.UpdatedMetadata {
			db.lastWrite = time.Now()
		}
	} else if _, ok := err.(*persistence.ConditionFailedError); ok {
		// tasks definitely were not created, restore the counter. For other errors tasks may or may not be created.
		// In those cases we keep the count incremented, hence it may be an overestimate.
		for i, update := range updates {
			db.subqueues[i].ApproximateBacklogCount -= int64(len(update.tasks))
		}
	}
	return createTasksResponse{bySubqueue: updates}, err
}

// CreateFairTasks creates a batch of given tasks for this task queue
func (db *taskQueueDB) CreateFairTasks(
	ctx context.Context,
	reqs []*writeTaskRequest,
) (createFairTasksResponse, error) {
	db.Lock()
	defer db.Unlock()

	if len(reqs) == 0 {
		return nil, nil
	}

	newTasks := make(createFairTasksResponse)
	newMaxLevel := make(map[int]fairLevel)
	allTasks := make([]*persistencespb.AllocatedTaskInfo, len(reqs))
	allSubqueues := make([]int, len(reqs))
	for i, req := range reqs {
		task := &persistencespb.AllocatedTaskInfo{
			TaskId:   req.id,
			TaskPass: req.pass,
			Data:     req.taskInfo,
		}
		allTasks[i] = task
		allSubqueues[i] = req.subqueue
		newTasks[req.subqueue] = append(newTasks[req.subqueue], task)
		newMaxLevel[req.subqueue] = newMaxLevel[req.subqueue].max(req.fairLevel)
	}

	for sq, tasks := range newTasks {
		db.subqueues[sq].ApproximateBacklogCount += int64(len(tasks))
	}

	// Unlike in CreateTasks, we can set the persisted FairMaxReadLevel before persisting.
	// This means that for stores that update metadata along with writing tasks (i.e. Cassandra),
	// the FairMaxReadLevel will be more up-to-date. The max read level is not used by
	// fairTaskReader, so there's no correctness issue with doing this.
	for sq, level := range newMaxLevel {
		db.subqueues[sq].FairMaxReadLevel = fairLevelFromProto(db.subqueues[sq].FairMaxReadLevel).max(level).toProto()
	}

	resp, err := db.store.CreateTasks(
		ctx,
		&persistence.CreateTasksRequest{
			TaskQueueInfo: &persistence.PersistedTaskQueueInfo{
				Data:    db.cachedQueueInfo(),
				RangeID: db.rangeID,
			},
			Tasks:     allTasks,
			Subqueues: allSubqueues,
		})

	if err == nil {
		// Only update lastWrite for persistence implementations that update metadata on CreateTasks.
		if resp.UpdatedMetadata {
			db.lastWrite = time.Now()
		}
	} else if _, ok := err.(*persistence.ConditionFailedError); ok {
		// Tasks definitely were not created, restore the counter. For other errors tasks may or may not be created.
		// In those cases we keep the count incremented, hence it may be an overestimate.
		// Don't bother restoring MaxReadLevel, it's okay if that's too high.
		for i, tasks := range newTasks {
			db.subqueues[i].ApproximateBacklogCount -= int64(len(tasks))
		}
	}
	return newTasks, err
}

// GetTasks returns a batch of tasks between the given range
func (db *taskQueueDB) GetTasks(
	ctx context.Context,
	subqueue int,
	inclusiveMinTaskID int64,
	exclusiveMaxTaskID int64,
	batchSize int,
) (*persistence.GetTasksResponse, error) {
	return db.store.GetTasks(ctx, &persistence.GetTasksRequest{
		NamespaceID:        db.queue.NamespaceId(),
		TaskQueue:          db.queue.PersistenceName(),
		TaskType:           db.queue.TaskType(),
		InclusiveMinTaskID: inclusiveMinTaskID,
		ExclusiveMaxTaskID: exclusiveMaxTaskID,
		Subqueue:           subqueue,
		PageSize:           batchSize,
	})
}

// GetFairTasks returns a batch of tasks after the given level
func (db *taskQueueDB) GetFairTasks(
	ctx context.Context,
	subqueue int,
	inclusiveMinLevel fairLevel,
	batchSize int,
) (*persistence.GetTasksResponse, error) {
	return db.store.GetTasks(ctx, &persistence.GetTasksRequest{
		NamespaceID:        db.queue.NamespaceId(),
		TaskQueue:          db.queue.PersistenceName(),
		TaskType:           db.queue.TaskType(),
		InclusiveMinPass:   inclusiveMinLevel.pass,
		InclusiveMinTaskID: inclusiveMinLevel.id,
		ExclusiveMaxTaskID: math.MaxInt64,
		Subqueue:           subqueue,
		PageSize:           batchSize,
		UseLimit:           true,
	})
}

// CompleteTasksLessThan deletes of tasks less than the given taskID. Limit is
// the upper bound of number of tasks that can be deleted by this method. It may
// or may not be honored
func (db *taskQueueDB) CompleteTasksLessThan(
	ctx context.Context,
	exclusiveMaxTaskID int64,
	limit int,
	subqueue int,
) (int, error) {
	n, err := db.store.CompleteTasksLessThan(ctx, &persistence.CompleteTasksLessThanRequest{
		NamespaceID:        db.queue.NamespaceId(),
		TaskQueueName:      db.queue.PersistenceName(),
		TaskType:           db.queue.TaskType(),
		ExclusiveMaxTaskID: exclusiveMaxTaskID,
		Subqueue:           subqueue,
		Limit:              limit,
	})
	if err != nil {
		db.logger.Error("Persistent store operation failure",
			tag.StoreOperationCompleteTasksLessThan,
			tag.Error(err),
			tag.TaskID(exclusiveMaxTaskID),
			tag.WorkflowTaskQueueType(db.queue.TaskType()),
			tag.WorkflowTaskQueueName(db.queue.PersistenceName()),
		)
	}
	return n, err
}

// CompleteFairTasksLessThan deletes of tasks less than the given taskID. Limit is
// the upper bound of number of tasks that can be deleted by this method. It may
// or may not be honored
func (db *taskQueueDB) CompleteFairTasksLessThan(
	ctx context.Context,
	exclusiveMaxLevel fairLevel,
	limit int,
	subqueue int,
) (int, error) {
	n, err := db.store.CompleteTasksLessThan(ctx, &persistence.CompleteTasksLessThanRequest{
		NamespaceID:        db.queue.NamespaceId(),
		TaskQueueName:      db.queue.PersistenceName(),
		TaskType:           db.queue.TaskType(),
		ExclusiveMaxPass:   exclusiveMaxLevel.pass,
		ExclusiveMaxTaskID: exclusiveMaxLevel.id,
		Subqueue:           subqueue,
		Limit:              limit,
	})
	if err != nil {
		db.logger.Error("Persistent store operation failure",
			tag.StoreOperationCompleteTasksLessThan,
			tag.Error(err),
			tag.AckLevel(exclusiveMaxLevel),
			tag.WorkflowTaskQueueType(db.queue.TaskType()),
			tag.WorkflowTaskQueueName(db.queue.PersistenceName()),
		)
	}
	return n, err
}

func (db *taskQueueDB) AllocateSubqueue(
	ctx context.Context,
	key *persistencespb.SubqueueKey,
) ([]persistencespb.SubqueueInfo, error) {
	db.Lock()
	defer db.Unlock()

	newSubqueue := db.newSubqueueLocked(key)
	db.subqueues = append(db.subqueues, newSubqueue)

	// ensure written to metadata before returning
	err := db.updateTaskQueueLocked(ctx, false)
	if err != nil {
		// If this was a conflict, caller will shut down partition. Otherwise, we don't know
		// for sure if this write made it to persistence or not. We should forget about the new
		// subqueue and let a future call to AllocateSubqueue add it again. If we crash and
		// reload, the new owner may see the subqueue present, which is also fine.
		db.subqueues = db.subqueues[:len(db.subqueues)-1]
		return nil, err
	}

	return db.cloneSubqueues(), nil
}

func (db *taskQueueDB) expiryTime() *timestamppb.Timestamp {
	switch db.queue.Partition().Kind() {
	case enumspb.TASK_QUEUE_KIND_NORMAL:
		return nil
	case enumspb.TASK_QUEUE_KIND_STICKY:
		return timestamppb.New(time.Now().Add(stickyTaskQueueTTL))
	default:
		panic(fmt.Sprintf("taskQueueDB encountered unknown task kind: %v", db.queue.Partition().Kind()))
	}
}

func (db *taskQueueDB) cachedQueueInfo() *persistencespb.TaskQueueInfo {
	infos := make([]*persistencespb.SubqueueInfo, len(db.subqueues))
	for i := range db.subqueues {
		infos[i] = &db.subqueues[i].SubqueueInfo
	}
	return &persistencespb.TaskQueueInfo{
		NamespaceId:             db.queue.NamespaceId(),
		Name:                    db.queue.PersistenceName(),
		TaskType:                db.queue.TaskType(),
		Kind:                    db.queue.Partition().Kind(),
		AckLevel:                db.subqueues[subqueueZero].AckLevel, // backwards compatibility
		ExpiryTime:              db.expiryTime(),
		LastUpdateTime:          timestamp.TimeNowPtrUtc(),
		ApproximateBacklogCount: db.subqueues[subqueueZero].ApproximateBacklogCount, // backwards compatibility
		Subqueues:               infos,
	}
}

// emitBacklogGaugesLocked emits the approximate_backlog_count, approximate_backlog_age_seconds, and the legacy
// task_lag_per_tl gauges. For these gauges to be emitted, BreakdownMetricsByTaskQueue and BreakdownMetricsByPartition
// should be enabled. Additionally, for versioned queues, BreakdownMetricsByBuildID should also be enabled.
func (db *taskQueueDB) emitBacklogGaugesLocked() {
	if !db.config.BreakdownMetricsByTaskQueue() ||
		!db.config.BreakdownMetricsByPartition() ||
		(db.queue.IsVersioned() && !db.config.BreakdownMetricsByBuildID()) {
		return
	}

	var approximateBacklogCount, totalLag int64
	var oldestTime time.Time
	for _, s := range db.subqueues {
		approximateBacklogCount += s.ApproximateBacklogCount
		oldestTime = minNonZeroTime(oldestTime, s.oldestTime)
		// note: this metric is only an estimation for the lag.
		// taskID in DB may not be continuous, especially when task list ownership changes.
		if s.FairMaxReadLevel != nil && s.FairAckLevel != nil {
			// TODO(fairness): this is not a good estimate of anything, we should probably just
			// get rid of this metric.
			totalLag += s.FairMaxReadLevel.TaskId - s.FairAckLevel.TaskId
		} else {
			totalLag += s.maxReadLevel - s.AckLevel
		}
	}

	metrics.ApproximateBacklogCount.With(db.metricsHandler).Record(float64(approximateBacklogCount))
	if oldestTime.IsZero() {
		metrics.ApproximateBacklogAgeSeconds.With(db.metricsHandler).Record(0)
	} else {
		metrics.ApproximateBacklogAgeSeconds.With(db.metricsHandler).Record(time.Since(oldestTime).Seconds())
	}
	metrics.TaskLagPerTaskQueueGauge.With(db.metricsHandler).Record(float64(totalLag))
}

func (db *taskQueueDB) ensureDefaultSubqueuesLocked(
	infos []*persistencespb.SubqueueInfo,
	initAckLevel int64,
	initApproxCount int64,
) []*dbSubqueue {
	// convert+copy protos to []*dbSubqueue
	subqueues := make([]*dbSubqueue, len(infos))
	for i, info := range infos {
		subqueues[i] = &dbSubqueue{}
		proto.Merge(&subqueues[i].SubqueueInfo, info)
	}

	// check for default priority and add if not present (this may be initializing subqueue 0)
	defKey := &persistencespb.SubqueueKey{
		Priority: defaultPriorityLevel(db.config.PriorityLevels()),
	}
	hasDefault := slices.ContainsFunc(subqueues, func(s *dbSubqueue) bool {
		return proto.Equal(s.Key, defKey)
	})
	if !hasDefault {
		subqueues = append(subqueues, db.newSubqueueLocked(defKey))
		// If we are transitioning from no-subqueues to subqueues, initialize subqueue 0 with
		// the ack level and approx count from TaskQueueInfo.
		if len(subqueues) == 1 {
			subqueues[subqueueZero].AckLevel = initAckLevel
			subqueues[subqueueZero].ApproximateBacklogCount = initApproxCount
		}
	}
	return subqueues
}

func (db *taskQueueDB) newSubqueueLocked(key *persistencespb.SubqueueKey) *dbSubqueue {
	// For fifo queues: start ack level + max read level just before the current block.
	// For fair queues: ack level and max read level don't matter here.
	initAckLevel := rangeIDToTaskIDBlock(db.rangeID, db.config.RangeSize).start - 1
	softassert.That(db.logger, initAckLevel >= 0, "initAckLevel should not be negative")

	s := &dbSubqueue{maxReadLevel: initAckLevel}
	s.Key = key
	s.AckLevel = initAckLevel
	return s
}

// clone db.subqueues so we can return it outside our lock
func (db *taskQueueDB) cloneSubqueues() []persistencespb.SubqueueInfo {
	infos := make([]persistencespb.SubqueueInfo, len(db.subqueues))
	for i := range db.subqueues {
		proto.Merge(&infos[i], &db.subqueues[i].SubqueueInfo)
	}
	return infos
}
