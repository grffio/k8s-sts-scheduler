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

func TestSTSScheduler_PreFilter(t *testing.T) {
	tests := []struct {
		name     string
		labels   statefulset.Labels
		pod      *v1.Pod
		wantCode framework.Code
	}{
		{
			name:   "Pod without StatefulSet owner",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
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
			name:   "Pod without required labels",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
					Labels: map[string]string{"test.io/other": "label"},
				},
			},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Pod with required labels",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
					Labels: map[string]string{"test.io/kind": "k8s-operator"},
				},
			},
			wantCode: framework.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := statefulset.STSScheduler{
				Labels: tt.labels,
			}

			_, status := ss.PreFilter(context.Background(), nil, tt.pod)
			assert.Equal(t, tt.wantCode, status.Code())
		})
	}
}

func TestSTSScheduler_Filter(t *testing.T) {
	tests := []struct {
		name     string
		labels   statefulset.Labels
		node     *v1.Node
		pod      *v1.Pod
		wantCode framework.Code
	}{
		{
			name:   "Node without required label",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
					Labels: map[string]string{
						"test.io/other": "0",
					},
				},
			},
			pod:      &v1.Pod{},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Node with invalid label value",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
					Labels: map[string]string{
						"test.io/node": "invalid",
					},
				},
			},
			pod:      &v1.Pod{},
			wantCode: framework.Error,
		},
		{
			name:   "Pod index invalid",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
					Labels: map[string]string{
						"test.io/node": "0",
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-invalid",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
					Labels: map[string]string{"test.io/kind": "k8s-operator"},
				},
			},
			wantCode: framework.Error,
		},
		{
			name:   "Node with wrong label value",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"test.io/node": "1",
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
					Labels: map[string]string{"test.io/kind": "k8s-operator"},
				},
			},
			wantCode: framework.Unschedulable,
		},
		{
			name:   "Node with correct label value",
			labels: statefulset.Labels{Pod: []string{"test.io/kind"}, Node: "test.io/node"},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-0",
					Labels: map[string]string{
						"test.io/node": "0",
					},
				},
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-0",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "sts1"},
					},
					Labels: map[string]string{"test.io/kind": "k8s-operator"},
				},
			},
			wantCode: framework.Success,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := statefulset.STSScheduler{
				Labels: tt.labels,
			}

			nodeInfo := framework.NewNodeInfo()
			nodeInfo.SetNode(tt.node)

			status := ss.Filter(context.Background(), nil, tt.pod, nodeInfo)
			assert.Equal(t, tt.wantCode, status.Code())
		})
	}
}
