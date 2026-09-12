package statefulset_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	fwk "k8s.io/kube-scheduler/framework"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsToRegister(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	events, err := contracts.enqueue.EventsToRegister(context.Background())
	require.NoError(t, err)
	require.Len(t, events, 2)

	registered := make(map[fwk.ClusterEvent]bool, len(events))
	for _, event := range events {
		assert.NotNil(t, event.QueueingHintFn)
		registered[event.Event] = true
	}

	assert.True(t, registered[fwk.ClusterEvent{
		Resource:   fwk.Node,
		ActionType: fwk.Add | fwk.UpdateNodeLabel,
	}])
	assert.True(t, registered[fwk.ClusterEvent{
		Resource:   fwk.TargetPod,
		ActionType: fwk.Update,
	}])
}

func TestNodeQueueingHint(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)
	hintFn := queueingHintForEvent(
		t,
		contracts.enqueue,
		fwk.ClusterEvent{Resource: fwk.Node, ActionType: fwk.Add | fwk.UpdateNodeLabel},
	)
	logger := klog.Background()
	podOrdinalThree := statefulSetPod("default", "db-3", "3")

	tests := []struct {
		name        string
		pod         *corev1.Pod
		oldObj      any
		newObj      any
		errContains string
		wantHint    fwk.QueueingHint
		wantErr     bool
	}{
		{
			name:        "wrong new object type queues conservatively",
			pod:         podOrdinalThree,
			newObj:      &corev1.Pod{},
			errContains: "expected *v1.Node",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:        "wrong old object type queues conservatively",
			pod:         podOrdinalThree,
			oldObj:      &corev1.Pod{},
			newObj:      ordinalNode("worker-3", "3"),
			wantErr:     true,
			wantHint:    fwk.Queue,
			errContains: "expected *v1.Node",
		},
		{
			name:        "nil new object queues conservatively",
			pod:         podOrdinalThree,
			newObj:      nil,
			errContains: "nil new object",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:     "non StatefulSet pod is ignored",
			pod:      plainPod("default", "job-0"),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "invalid pod ordinal cannot be fixed by node change",
			pod:      statefulSetPod("default", "db-x", "bad"),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "new node without ordinal is irrelevant",
			pod:      podOrdinalThree,
			newObj:   node("worker", map[string]string{}),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "new node with invalid ordinal is irrelevant",
			pod:      podOrdinalThree,
			newObj:   ordinalNode("worker", "bad"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "new node with another ordinal is irrelevant",
			pod:      podOrdinalThree,
			newObj:   ordinalNode("worker-4", "4"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "matching node add queues pod",
			pod:      podOrdinalThree,
			oldObj:   nil,
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.Queue,
		},
		{
			name:     "non-matching to matching label update queues pod",
			pod:      podOrdinalThree,
			oldObj:   ordinalNode("worker-3", "4"),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.Queue,
		},
		{
			name:     "missing to matching label update queues pod",
			pod:      podOrdinalThree,
			oldObj:   node("worker-3", map[string]string{}),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.Queue,
		},
		{
			name:     "invalid to matching label update queues pod",
			pod:      podOrdinalThree,
			oldObj:   ordinalNode("worker-3", "bad"),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.Queue,
		},
		{
			name:     "already matching node remains skipped",
			pod:      podOrdinalThree,
			oldObj:   ordinalNode("worker-3", "3"),
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.QueueSkip,
		},
		{
			name: "tombstoned matching old node remains skipped",
			pod:  podOrdinalThree,
			oldObj: cache.DeletedFinalStateUnknown{
				Obj: ordinalNode("worker-3", "3"),
			},
			newObj:   ordinalNode("worker-3", "3"),
			wantHint: fwk.QueueSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := hintFn(logger, tt.pod, tt.oldObj, tt.newObj)
			assert.Equal(t, tt.wantHint, got)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestTargetPodQueueingHint(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)
	hintFn := queueingHintForEvent(
		t,
		contracts.enqueue,
		fwk.ClusterEvent{Resource: fwk.TargetPod, ActionType: fwk.Update},
	)
	logger := klog.Background()
	queuedPod := statefulSetPod("default", "queued-db-2", "2")

	tests := []struct {
		name        string
		oldObj      any
		newObj      any
		errContains string
		wantHint    fwk.QueueingHint
		wantErr     bool
	}{
		{
			name:        "wrong new object type queues conservatively",
			oldObj:      plainPod("default", "pod"),
			newObj:      &corev1.Node{},
			errContains: "expected *v1.Pod",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:        "wrong old object type queues conservatively",
			oldObj:      &corev1.Node{},
			newObj:      plainPod("default", "pod"),
			errContains: "expected *v1.Pod",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:        "nil old object is invalid for update",
			oldObj:      nil,
			newObj:      plainPod("default", "pod"),
			errContains: "requires old and new objects",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:        "nil new object is invalid for update",
			oldObj:      plainPod("default", "pod"),
			newObj:      nil,
			errContains: "requires old and new objects",
			wantHint:    fwk.Queue,
			wantErr:     true,
		},
		{
			name:     "StatefulSet controller removed queues pod",
			oldObj:   statefulSetPod("default", "db-2", "2"),
			newObj:   plainPod("default", "db-2"),
			wantHint: fwk.Queue,
		},
		{
			name:     "non StatefulSet update is skipped",
			oldObj:   plainPod("default", "job-0"),
			newObj:   plainPod("default", "job-0"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "transition to invalid StatefulSet remains skipped",
			oldObj:   plainPod("default", "db-x"),
			newObj:   statefulSetPod("default", "db-x", "bad"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "transition to valid StatefulSet queues",
			oldObj:   plainPod("default", "db-2"),
			newObj:   statefulSetPod("default", "db-2", "2"),
			wantHint: fwk.Queue,
		},
		{
			name:     "invalid ordinal becoming valid queues",
			oldObj:   statefulSetPod("default", "db-x", "bad"),
			newObj:   statefulSetPod("default", "db-2", "2"),
			wantHint: fwk.Queue,
		},
		{
			name:     "ordinal change queues",
			oldObj:   statefulSetPod("default", "db-2", "2"),
			newObj:   statefulSetPod("default", "db-3", "3"),
			wantHint: fwk.Queue,
		},
		{
			name:     "same ordinal is skipped",
			oldObj:   statefulSetPod("default", "db-2", "2"),
			newObj:   statefulSetPod("default", "db-2", "2"),
			wantHint: fwk.QueueSkip,
		},
		{
			name:     "valid ordinal becoming invalid is skipped",
			oldObj:   statefulSetPod("default", "db-2", "2"),
			newObj:   statefulSetPod("default", "db-x", "bad"),
			wantHint: fwk.QueueSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := hintFn(logger, queuedPod, tt.oldObj, tt.newObj)
			assert.Equal(t, tt.wantHint, got)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func queueingHintForEvent(t *testing.T, plugin fwk.EnqueueExtensions, want fwk.ClusterEvent) fwk.QueueingHintFn {
	t.Helper()

	events, err := plugin.EventsToRegister(context.Background())
	require.NoError(t, err)

	for _, event := range events {
		if event.Event == want {
			require.NotNil(t, event.QueueingHintFn)
			return event.QueueingHintFn
		}
	}

	require.FailNow(t, "queueing hint event is not registered", "event: %#v", want)
	return nil
}
