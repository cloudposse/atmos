package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/dependency"
)

var (
	ErrNilGraph      = errors.New("scheduler graph cannot be nil")
	ErrNilDispatcher = errors.New("scheduler dispatcher cannot be nil")
	ErrNodeSkipped   = errors.New("scheduler node skipped")
	ErrNodeNotFound  = errors.New("scheduler node not found")
	ErrInvalidGraph  = errors.New("scheduler graph is invalid")
	ErrInvalidWorker = errors.New("scheduler max concurrency must be greater than zero")
)

// Status describes the scheduler outcome for a node.
type Status string

const (
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
)

// Dispatcher executes one scheduler node. Tool-specific execution belongs behind
// this interface so the scheduler remains component-type agnostic.
type Dispatcher interface {
	Dispatch(ctx context.Context, node *dependency.Node) (Result, error)
}

// DispatcherFunc adapts a function into a Dispatcher.
type DispatcherFunc func(ctx context.Context, node *dependency.Node) (Result, error)

// Dispatch calls f(ctx, node).
func (f DispatcherFunc) Dispatch(ctx context.Context, node *dependency.Node) (Result, error) {
	return f(ctx, node)
}

// Result is the deterministic per-node outcome returned in AggregateResult.
type Result struct {
	NodeID string
	Node   dependency.Node
	Status Status
	Value  any
	Err    error
}

// Success reports whether the node completed successfully.
func (r Result) Success() bool {
	return r.Status == StatusSucceeded && r.Err == nil
}

// AggregateResult contains one result per graph node in stable topological order.
type AggregateResult struct {
	Results []Result
	Err     error
}

// Success reports whether all scheduled nodes completed successfully.
func (r *AggregateResult) Success() bool {
	return r != nil && r.Err == nil
}

// ResultFor returns the result for a node ID.
func (r *AggregateResult) ResultFor(nodeID string) (Result, bool) {
	if r == nil {
		return Result{}, false
	}
	for _, result := range r.Results {
		if result.NodeID == nodeID {
			return result, true
		}
	}
	return Result{}, false
}

// Scheduler manages ready-queue DAG execution with a bounded worker pool.
type Scheduler struct {
	graph          *dependency.Graph
	dispatcher     Dispatcher
	maxConcurrency int
	failFast       bool
	onNodeStart    func(*dependency.Node)
	onNodeComplete func(*dependency.Node, Result)
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithMaxConcurrency sets the number of workers allowed to run at once.
func WithMaxConcurrency(maxConcurrency int) Option {
	return func(s *Scheduler) {
		s.maxConcurrency = maxConcurrency
	}
}

// WithFailFast controls whether the scheduler stops scheduling after a node error.
func WithFailFast(failFast bool) Option {
	return func(s *Scheduler) {
		s.failFast = failFast
	}
}

// WithNodeStartHook registers a callback before a node is dispatched.
func WithNodeStartHook(hook func(*dependency.Node)) Option {
	return func(s *Scheduler) {
		s.onNodeStart = hook
	}
}

// WithNodeCompleteHook registers a callback after a node dispatch finishes.
func WithNodeCompleteHook(hook func(*dependency.Node, Result)) Option {
	return func(s *Scheduler) {
		s.onNodeComplete = hook
	}
}

// New creates a scheduler for graph and dispatcher.
func New(graph *dependency.Graph, dispatcher Dispatcher, opts ...Option) *Scheduler {
	s := &Scheduler{
		graph:          graph,
		dispatcher:     dispatcher,
		maxConcurrency: 1,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run executes the graph with ready-queue scheduling.
func (s *Scheduler) Run(ctx context.Context) *AggregateResult {
	if ctx == nil {
		ctx = context.Background()
	}

	orderedIDs, err := s.validate()
	if err != nil {
		return &AggregateResult{Err: err}
	}
	if len(orderedIDs) == 0 {
		return &AggregateResult{}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ready := newReadyQueue(rootNodeIDs(s.graph))
	inDegree := nodeInDegrees(s.graph)
	states := make(map[string]nodeState, len(s.graph.Nodes))
	results := make(map[string]Result, len(s.graph.Nodes))
	for id := range s.graph.Nodes {
		states[id] = statePending
	}

	workCh := make(chan string)
	eventCh := make(chan runEvent)
	workerCount := min(s.maxConcurrency, len(s.graph.Nodes))

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go s.worker(runCtx, workCh, eventCh, &workers)
	}

	running := 0
	finished := 0
	stopping := false

	for finished < len(s.graph.Nodes) {
		var nextID string
		var out chan<- string
		if !stopping && running < workerCount && ready.Len() > 0 {
			nextID = ready.Peek()
			for ready.Len() > 0 && states[nextID] != statePending {
				ready.Pop()
				if ready.Len() > 0 {
					nextID = ready.Peek()
				}
			}
			if ready.Len() > 0 && states[nextID] == statePending {
				out = workCh
			}
		}
		var done <-chan struct{}
		if !stopping {
			done = runCtx.Done()
		}

		select {
		case out <- nextID:
			ready.Pop()
			states[nextID] = stateRunning
			running++
		case event := <-eventCh:
			running--
			finished++
			s.recordEvent(event, states, results, inDegree, ready)
			if event.result.Status == StatusFailed {
				if s.failFast {
					stopping = true
					cancel()
					finished += skipPending(states, results, s.graph, fmt.Errorf("%w: fail-fast after %s failed", ErrNodeSkipped, event.nodeID))
				} else {
					finished += skipBlocked(event.nodeID, states, results, s.graph, fmt.Errorf("%w: dependency %s failed", ErrNodeSkipped, event.nodeID))
				}
			}
		case <-done:
			stopping = true
			finished += skipPending(states, results, s.graph, runCtx.Err())
		}
	}

	close(workCh)
	workers.Wait()

	aggregate := &AggregateResult{
		Results: orderedResults(orderedIDs, results),
	}
	aggregate.Err = aggregateError(aggregate.Results)
	return aggregate
}

func (s *Scheduler) validate() ([]string, error) {
	if s == nil {
		return nil, ErrNilGraph
	}
	if s.graph == nil {
		return nil, ErrNilGraph
	}
	if s.dispatcher == nil {
		return nil, ErrNilDispatcher
	}
	if s.maxConcurrency <= 0 {
		return nil, ErrInvalidWorker
	}
	if hasCycle, cyclePath := s.graph.HasCycles(); hasCycle {
		return nil, fmt.Errorf("%w: %w: %v", ErrInvalidGraph, dependency.ErrCircularDependency, cyclePath)
	}
	return topologicalNodeIDs(s.graph)
}

func (s *Scheduler) worker(ctx context.Context, workCh <-chan string, eventCh chan<- runEvent, workers *sync.WaitGroup) {
	defer workers.Done()
	for nodeID := range workCh {
		node, ok := s.graph.GetNode(nodeID)
		if !ok {
			eventCh <- runEvent{nodeID: nodeID, result: failedResult(nodeID, dependency.Node{ID: nodeID}, ErrNodeNotFound)}
			continue
		}
		if s.onNodeStart != nil {
			s.onNodeStart(node)
		}

		result, err := s.dispatcher.Dispatch(ctx, node)
		result = normalizeResult(node, result, err)

		if s.onNodeComplete != nil {
			s.onNodeComplete(node, result)
		}
		eventCh <- runEvent{nodeID: nodeID, result: result}
	}
}

func (s *Scheduler) recordEvent(event runEvent, states map[string]nodeState, results map[string]Result, inDegree map[string]int, ready *readyQueue) {
	states[event.nodeID] = stateFinished
	results[event.nodeID] = event.result

	if event.result.Status != StatusSucceeded {
		return
	}

	node := s.graph.Nodes[event.nodeID]
	for _, dependentID := range sortedCopy(node.Dependents) {
		if states[dependentID] != statePending {
			continue
		}
		inDegree[dependentID]--
		if inDegree[dependentID] == 0 {
			ready.Push(dependentID)
		}
	}
}

func normalizeResult(node *dependency.Node, result Result, err error) Result {
	if result.NodeID == "" {
		result.NodeID = node.ID
	}
	result.Node = *node
	if err != nil {
		result.Err = err
	}
	if result.Err != nil {
		result.Status = StatusFailed
		return result
	}
	if result.Status == "" {
		result.Status = StatusSucceeded
	}
	return result
}

func failedResult(nodeID string, node dependency.Node, err error) Result {
	return Result{
		NodeID: nodeID,
		Node:   node,
		Status: StatusFailed,
		Err:    err,
	}
}

func skippedResult(node *dependency.Node, err error) Result {
	return Result{
		NodeID: node.ID,
		Node:   *node,
		Status: StatusSkipped,
		Err:    err,
	}
}

func skipPending(states map[string]nodeState, results map[string]Result, graph *dependency.Graph, err error) int {
	skipped := 0
	for _, nodeID := range sortedNodeIDs(graph) {
		if states[nodeID] != statePending {
			continue
		}
		states[nodeID] = stateFinished
		results[nodeID] = skippedResult(graph.Nodes[nodeID], err)
		skipped++
	}
	return skipped
}

func skipBlocked(failedID string, states map[string]nodeState, results map[string]Result, graph *dependency.Graph, err error) int {
	skipped := 0
	var visit func(string)
	visit = func(nodeID string) {
		node := graph.Nodes[nodeID]
		for _, dependentID := range sortedCopy(node.Dependents) {
			if states[dependentID] != statePending {
				continue
			}
			states[dependentID] = stateFinished
			results[dependentID] = skippedResult(graph.Nodes[dependentID], err)
			skipped++
			visit(dependentID)
		}
	}
	visit(failedID)
	return skipped
}

func aggregateError(results []Result) error {
	errs := make([]error, 0)
	for _, result := range results {
		if (result.Status == StatusFailed || result.Status == StatusSkipped) && result.Err != nil {
			errs = append(errs, result.Err)
		}
	}
	return errors.Join(errs...)
}

func orderedResults(orderedIDs []string, results map[string]Result) []Result {
	ordered := make([]Result, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if result, ok := results[id]; ok {
			ordered = append(ordered, result)
		}
	}
	return ordered
}

func topologicalNodeIDs(graph *dependency.Graph) ([]string, error) {
	inDegree := nodeInDegrees(graph)
	ready := newReadyQueue(rootNodeIDs(graph))
	ordered := make([]string, 0, len(graph.Nodes))

	for ready.Len() > 0 {
		nodeID := ready.Pop()
		ordered = append(ordered, nodeID)
		for _, dependentID := range sortedCopy(graph.Nodes[nodeID].Dependents) {
			inDegree[dependentID]--
			if inDegree[dependentID] == 0 {
				ready.Push(dependentID)
			}
		}
	}

	if len(ordered) != len(graph.Nodes) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidGraph, dependency.ErrCircularDependency)
	}
	return ordered, nil
}

func nodeInDegrees(graph *dependency.Graph) map[string]int {
	inDegree := make(map[string]int, len(graph.Nodes))
	for id, node := range graph.Nodes {
		inDegree[id] = len(node.Dependencies)
	}
	return inDegree
}

func rootNodeIDs(graph *dependency.Graph) []string {
	roots := make([]string, 0)
	for id, node := range graph.Nodes {
		if len(node.Dependencies) == 0 {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)
	return roots
}

func sortedNodeIDs(graph *dependency.Graph) []string {
	ids := make([]string, 0, len(graph.Nodes))
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedCopy(values []string) []string {
	copied := append([]string{}, values...)
	sort.Strings(copied)
	return copied
}

type runEvent struct {
	nodeID string
	result Result
}

type nodeState int

const (
	statePending nodeState = iota
	stateRunning
	stateFinished
)

type readyQueue struct {
	ids []string
}

func newReadyQueue(ids []string) *readyQueue {
	q := &readyQueue{}
	for _, id := range ids {
		q.Push(id)
	}
	return q
}

func (q *readyQueue) Len() int {
	return len(q.ids)
}

func (q *readyQueue) Push(id string) {
	q.ids = append(q.ids, id)
	sort.Strings(q.ids)
}

func (q *readyQueue) Peek() string {
	return q.ids[0]
}

func (q *readyQueue) Pop() string {
	id := q.ids[0]
	q.ids = q.ids[1:]
	return id
}
