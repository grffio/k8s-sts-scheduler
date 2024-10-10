package statefulset_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grffio/k8s-sts-scheduler/pkg/statefulset"
)

func TestStatefulSetScheduler_PreEnqueue(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		wantCode framework.Code
	}{
		{
			name: "Pod without StatefulSet owner",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "rs1"},
					},
				},
			},
			wantCode: framework.UnschedulableAndUnresolvable,
		},
		{
			name: "Pod with StatefulSet owner",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
				},
			},
			wantCode: framework.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := statefulset.Scheduler{}

			status := ss.PreEnqueue(context.Background(), tt.pod)
			assert.Equal(t, tt.wantCode, status.Code())
		})
	}
}

func TestStatefulSetScheduler_PreFilter(t *testing.T) {
	tests := []struct {
		name     string
		labels   statefulset.Labels
		pod      *v1.Pod
		wantCode framework.Code
	}{
		{
			name:   "Pod without required labels",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					Labels: map[string]string{
						"test.io/other": "label",
					},
				},
			},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Pod with required labels",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					Labels: map[string]string{
						"test.io/kind": "k8s-operator",
					},
				},
			},
			wantCode: framework.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := statefulset.Scheduler{
				Labels: tt.labels,
			}

			_, status := ss.PreFilter(context.Background(), nil, tt.pod)
			assert.Equal(t, tt.wantCode, status.Code())
		})
	}
}

func TestStatefulSetScheduler_Filter(t *testing.T) {
	tests := []struct {
		name     string
		labels   statefulset.Labels
		node     *v1.Node
		pod      *v1.Pod
		wantCode framework.Code
	}{
		{
			name:   "Node without required label",
			labels: statefulset.Labels{Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"test.io/other": "0"},
				},
			},
			pod:      &v1.Pod{},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Node with invalid label value",
			labels: statefulset.Labels{Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"test.io/node": "invalid"},
				},
			},
			pod:      &v1.Pod{},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Pod with invalid ordinal",
			labels: statefulset.Labels{Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"test.io/node": "0"},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-invalid",
				},
			},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Node label value does not match pod ordinal",
			labels: statefulset.Labels{Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-1",
					Labels: map[string]string{"test.io/node": "1"},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
				},
			},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Node label value matches pod ordinal",
			labels: statefulset.Labels{Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "node-0",
					Labels: map[string]string{"test.io/node": "0"},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
				},
			},
			wantCode: framework.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := statefulset.Scheduler{
				Labels: tt.labels,
			}

			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(tt.node)

			status := ss.Filter(context.Background(), nil, tt.pod, nodeInfo)
			assert.Equal(t, tt.wantCode, status.Code())
		})
	}
}
