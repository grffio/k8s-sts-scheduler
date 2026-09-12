package statefulset_test

import (
	"context"
	"errors"
	"math"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	fwk "k8s.io/kube-scheduler/framework"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignPod_OwnerContract(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)
	controller := true
	notController := false

	tests := []struct {
		name          string
		pod           *corev1.Pod
		wantFragments []fwk.SignFragment
		wantCode      fwk.Code
	}{
		{
			name:     "nil pod is ignored",
			pod:      nil,
			wantCode: fwk.Success,
		},
		{
			name:     "pod without owner is ignored",
			pod:      plainPod("default", "job-0"),
			wantCode: fwk.Success,
		},
		{
			name: "StatefulSet non-controller owner is ignored",
			pod: podWithOwner(
				"default",
				"db-3",
				appsv1.SchemeGroupVersion.String(),
				"StatefulSet",
				&notController,
				"3",
			),
			wantCode: fwk.Success,
		},
		{
			name: "wrong controller kind is ignored",
			pod: podWithOwner(
				"default",
				"db-3",
				appsv1.SchemeGroupVersion.String(),
				"ReplicaSet",
				&controller,
				"3",
			),
			wantCode: fwk.Success,
		},
		{
			name: "wrong controller API version is ignored",
			pod: podWithOwner(
				"default",
				"db-3",
				"apps/v1beta1",
				"StatefulSet",
				&controller,
				"3",
			),
			wantCode: fwk.Success,
		},
		{
			name: "StatefulSet controller is handled",
			pod: podWithOwner(
				"default",
				"db-3",
				appsv1.SchemeGroupVersion.String(),
				"StatefulSet",
				&controller,
				"3",
			),
			wantFragments: []fwk.SignFragment{
				{
					Key:   "k8s-sts-scheduler.grffio.github.com/pod-ordinal",
					Value: int32(3),
				},
			},
			wantCode: fwk.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fragments, status := contracts.sign.SignPod(context.Background(), tt.pod)
			assertStatus(t, status, tt.wantCode, "")
			assert.Equal(t, tt.wantFragments, fragments)
		})
	}
}

func TestSignPod_OrdinalContract(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	tests := []struct {
		name        string
		pod         *corev1.Pod
		wantOrdinal *int32
		wantReason  string
		wantCode    fwk.Code
	}{
		{
			name:       "missing ordinal label",
			pod:        statefulSetPodWithoutOrdinal("default", "db-0"),
			wantReason: "missing required ordinal label",
			wantCode:   fwk.Unschedulable,
		},
		{
			name:        "zero",
			pod:         statefulSetPod("default", "db-0", "0"),
			wantOrdinal: int32Ptr(0),
			wantCode:    fwk.Success,
		},
		{
			name:        "positive",
			pod:         statefulSetPod("default", "db-42", "42"),
			wantOrdinal: int32Ptr(42),
			wantCode:    fwk.Success,
		},
		{
			name:        "max int32",
			pod:         statefulSetPod("default", "db-max", "2147483647"),
			wantOrdinal: int32Ptr(math.MaxInt32),
			wantCode:    fwk.Success,
		},
		{
			name:        "leading zeroes",
			pod:         statefulSetPod("default", "db-7", "0007"),
			wantOrdinal: int32Ptr(7),
			wantCode:    fwk.Success,
		},
		{
			name:       "empty value",
			pod:        statefulSetPod("default", "db-empty", ""),
			wantReason: "invalid ordinal",
			wantCode:   fwk.Unschedulable,
		},
		{
			name:       "negative",
			pod:        statefulSetPod("default", "db-negative", "-1"),
			wantCode:   fwk.Unschedulable,
			wantReason: "invalid ordinal",
		},
		{
			name:       "int32 overflow",
			pod:        statefulSetPod("default", "db-overflow", "2147483648"),
			wantCode:   fwk.Unschedulable,
			wantReason: "invalid ordinal",
		},
		{
			name:       "fraction",
			pod:        statefulSetPod("default", "db-fraction", "1.5"),
			wantReason: "invalid ordinal",
			wantCode:   fwk.Unschedulable,
		},
		{
			name:       "leading whitespace",
			pod:        statefulSetPod("default", "db-space", " 1"),
			wantReason: "invalid ordinal",
			wantCode:   fwk.Unschedulable,
		},
		{
			name:       "hexadecimal",
			pod:        statefulSetPod("default", "db-hex", "0x10"),
			wantReason: "invalid ordinal",
			wantCode:   fwk.Unschedulable,
		},
		{
			name:       "text",
			pod:        statefulSetPod("default", "db-text", "one"),
			wantReason: "invalid ordinal",
			wantCode:   fwk.Unschedulable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fragments, status := contracts.sign.SignPod(context.Background(), tt.pod)
			assertStatus(t, status, tt.wantCode, tt.wantReason)

			if tt.wantOrdinal == nil {
				assert.Nil(t, fragments)
				return
			}

			require.Len(t, fragments, 1)
			assert.Equal(t, "k8s-sts-scheduler.grffio.github.com/pod-ordinal", fragments[0].Key)
			assert.Equal(t, *tt.wantOrdinal, fragments[0].Value)
		})
	}
}

func TestPreFilter(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	tests := []struct {
		name       string
		pod        *corev1.Pod
		state      fwk.CycleState
		wantReason string
		wantCode   fwk.Code
	}{
		{
			name:     "nil pod is skipped",
			pod:      nil,
			state:    newCycleState(),
			wantCode: fwk.Skip,
		},
		{
			name:     "non StatefulSet pod is skipped",
			pod:      plainPod("default", "job-0"),
			state:    newCycleState(),
			wantCode: fwk.Skip,
		},
		{
			name:       "missing ordinal is unresolvable",
			pod:        statefulSetPodWithoutOrdinal("default", "db-0"),
			state:      newCycleState(),
			wantReason: "missing required ordinal label",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "invalid ordinal is unresolvable",
			pod:        statefulSetPod("default", "db-x", "bad"),
			state:      newCycleState(),
			wantReason: "invalid ordinal",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "nil cycle state is internal error",
			pod:        statefulSetPod("default", "db-4", "4"),
			state:      nil,
			wantReason: "cycle state is nil",
			wantCode:   fwk.Error,
		},
		{
			name:     "valid StatefulSet pod succeeds",
			pod:      statefulSetPod("default", "db-4", "4"),
			state:    newCycleState(),
			wantCode: fwk.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, status := contracts.preFilter.PreFilter(
				context.Background(),
				tt.state,
				tt.pod,
				nil,
			)

			assert.Nil(t, result)
			assertStatus(t, status, tt.wantCode, tt.wantReason)
		})
	}
}

func TestPreFilterFilter_StateHandoff(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)
	pod := statefulSetPod("default", "db-2", "2")
	state := newCycleState()

	_, status := contracts.preFilter.PreFilter(context.Background(), state, pod, nil)
	assertStatus(t, status, fwk.Success, "")

	// Mutating the Pod after PreFilter proves Filter consumes the scheduling-cycle
	// state instead of deriving the ordinal again.
	pod.Labels[appsv1.PodIndexLabel] = "9"

	status = contracts.filter.Filter(
		context.Background(),
		state,
		pod,
		nodeInfo(ordinalNode("worker-2", "2")),
	)
	assertStatus(t, status, fwk.Success, "")

	status = contracts.filter.Filter(
		context.Background(),
		state,
		pod,
		nodeInfo(ordinalNode("worker-9", "9")),
	)
	assertStatus(t, status, fwk.UnschedulableAndUnresolvable, "pod requires ordinal 2")
}

func TestPreFilterFilter_ClonedCycleState(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)
	pod := statefulSetPod("default", "db-6", "6")
	state := newCycleState()

	_, status := contracts.preFilter.PreFilter(context.Background(), state, pod, nil)
	assertStatus(t, status, fwk.Success, "")

	cloned := state.Clone()
	require.NotNil(t, cloned)

	pod.Labels[appsv1.PodIndexLabel] = "8"
	status = contracts.filter.Filter(
		context.Background(),
		cloned,
		pod,
		nodeInfo(ordinalNode("worker-6", "6")),
	)
	assertStatus(t, status, fwk.Success, "")
}

func TestFilter(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	canceledCtx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("scheduling cycle canceled"))

	tests := []struct {
		name       string
		ctx        context.Context
		pod        *corev1.Pod
		nodeInfo   fwk.NodeInfo
		wantReason string
		wantCode   fwk.Code
	}{
		{
			name:       "canceled context short-circuits filtering",
			ctx:        canceledCtx,
			pod:        statefulSetPod("default", "db-1", "1"),
			nodeInfo:   nodeInfo(ordinalNode("worker-1", "1")),
			wantReason: "scheduling cycle canceled",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:     "matching node succeeds",
			ctx:      context.Background(),
			pod:      statefulSetPod("default", "db-2", "2"),
			nodeInfo: nodeInfo(ordinalNode("worker-2", "2")),
			wantCode: fwk.Success,
		},
		{
			name:       "different node ordinal is rejected",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(ordinalNode("worker-3", "3")),
			wantReason: `node "worker-3" has ordinal 3, pod requires ordinal 2`,
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "node without configured label is rejected",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(node("worker-2", map[string]string{})),
			wantReason: "missing required ordinal label",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "node with empty ordinal is rejected",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(ordinalNode("worker-2", "")),
			wantReason: "invalid ordinal",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "node with negative ordinal is rejected",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(ordinalNode("worker-2", "-1")),
			wantReason: "invalid ordinal",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "node ordinal overflow is rejected",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(ordinalNode("worker-2", "2147483648")),
			wantReason: "invalid ordinal",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "nil NodeInfo is internal error",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nil,
			wantReason: "without a Node",
			wantCode:   fwk.Error,
		},
		{
			name:       "NodeInfo without Node is internal error",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-2", "2"),
			nodeInfo:   nodeInfo(nil),
			wantReason: "without a Node",
			wantCode:   fwk.Error,
		},
		{
			name:       "invalid StatefulSet ordinal is unresolvable",
			ctx:        context.Background(),
			pod:        statefulSetPod("default", "db-x", "bad"),
			nodeInfo:   nodeInfo(ordinalNode("worker-2", "2")),
			wantReason: "invalid ordinal",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:       "missing StatefulSet ordinal is unresolvable",
			ctx:        context.Background(),
			pod:        statefulSetPodWithoutOrdinal("default", "db-x"),
			nodeInfo:   nodeInfo(ordinalNode("worker-2", "2")),
			wantReason: "missing required ordinal label",
			wantCode:   fwk.UnschedulableAndUnresolvable,
		},
		{
			name:     "non StatefulSet pod is ignored without PreFilter",
			ctx:      context.Background(),
			pod:      plainPod("default", "job-0"),
			wantCode: fwk.Success,
			nodeInfo: nodeInfo(ordinalNode("worker-2", "2")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status := contracts.filter.Filter(
				tt.ctx,
				newCycleState(),
				tt.pod,
				tt.nodeInfo,
			)
			assertStatus(t, status, tt.wantCode, tt.wantReason)
		})
	}
}

func TestFilter_WithoutPreFilter_OrdinalParsing(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	tests := []struct {
		name        string
		podOrdinal  string
		nodeOrdinal string
		wantReason  string
		wantCode    fwk.Code
	}{
		{
			name:        "leading zeroes normalize numerically",
			podOrdinal:  "0007",
			nodeOrdinal: "7",
			wantCode:    fwk.Success,
		},
		{
			name:        "max int32",
			podOrdinal:  "2147483647",
			nodeOrdinal: "2147483647",
			wantCode:    fwk.Success,
		},
		{
			name:        "same textual shape is not required",
			podOrdinal:  "7",
			nodeOrdinal: "0007",
			wantCode:    fwk.Success,
		},
		{
			name:        "different numeric ordinals reject",
			podOrdinal:  "7",
			nodeOrdinal: "8",
			wantReason:  "pod requires ordinal 7",
			wantCode:    fwk.UnschedulableAndUnresolvable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status := contracts.filter.Filter(
				context.Background(),
				newCycleState(),
				statefulSetPod("default", "db", tt.podOrdinal),
				nodeInfo(ordinalNode("worker", tt.nodeOrdinal)),
			)
			assertStatus(t, status, tt.wantCode, tt.wantReason)
		})
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
