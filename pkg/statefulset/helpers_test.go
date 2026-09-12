package statefulset_test

import (
	"context"
	"encoding/json"
	"testing"

	statefulset "github.com/grffio/k8s-sts-scheduler/pkg/statefulset"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fwk "k8s.io/kube-scheduler/framework"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNodeOrdinalLabel = "topology.example.com/ordinal"

type pluginContracts struct {
	plugin    fwk.Plugin
	sign      fwk.SignPlugin
	preFilter fwk.PreFilterPlugin
	filter    fwk.FilterPlugin
	enqueue   fwk.EnqueueExtensions
}

func newPlugin(t *testing.T, nodeOrdinalLabel string) pluginContracts {
	t.Helper()

	config := runtimeConfig(t, statefulset.Args{
		NodeOrdinalLabel: nodeOrdinalLabel,
	})

	p, err := statefulset.New(context.Background(), config, nil)
	require.NoError(t, err)

	sign, ok := p.(fwk.SignPlugin)
	require.True(t, ok, "plugin %T does not implement fwk.SignPlugin", p)

	preFilter, ok := p.(fwk.PreFilterPlugin)
	require.True(t, ok, "plugin %T does not implement fwk.PreFilterPlugin", p)

	filter, ok := p.(fwk.FilterPlugin)
	require.True(t, ok, "plugin %T does not implement fwk.FilterPlugin", p)

	enqueue, ok := p.(fwk.EnqueueExtensions)
	require.True(t, ok, "plugin %T does not implement fwk.EnqueueExtensions", p)

	return pluginContracts{
		plugin:    p,
		sign:      sign,
		preFilter: preFilter,
		filter:    filter,
		enqueue:   enqueue,
	}
}

func runtimeConfig(t *testing.T, args statefulset.Args) *runtime.Unknown {
	t.Helper()

	raw, err := json.Marshal(args)
	require.NoError(t, err)

	return &runtime.Unknown{Raw: raw}
}

func statefulSetPod(namespace, name, ordinal string) *corev1.Pod {
	controller := true

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				appsv1.PodIndexLabel: ordinal,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: appsv1.SchemeGroupVersion.String(),
					Kind:       "StatefulSet",
					Name:       "db",
					Controller: &controller,
				},
			},
		},
	}
}

func statefulSetPodWithoutOrdinal(namespace, name string) *corev1.Pod {
	pod := statefulSetPod(namespace, name, "")
	delete(pod.Labels, appsv1.PodIndexLabel)
	return pod
}

//nolint:unparam // namespace is intentionally parameterized for test helper readability and reuse.
func plainPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

//nolint:unparam // namespace and name are intentionally parameterized for test helper readability and reuse.
func podWithOwner(namespace, name, apiVersion, kind string, controller *bool, ordinal string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				appsv1.PodIndexLabel: ordinal,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: apiVersion,
					Kind:       kind,
					Name:       "owner",
					Controller: controller,
				},
			},
		},
	}
}

func node(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func ordinalNode(name, ordinal string) *corev1.Node {
	return node(name, map[string]string{
		testNodeOrdinalLabel: ordinal,
	})
}

func nodeInfo(n *corev1.Node) fwk.NodeInfo {
	info := schedulerframework.NewNodeInfo()
	if n != nil {
		info.SetNode(n)
	}
	return info
}

func newCycleState() fwk.CycleState {
	return schedulerframework.NewCycleState()
}

func assertStatus(t *testing.T, got *fwk.Status, wantCode fwk.Code, reasonContains string) {
	t.Helper()

	assert.Equal(t, wantCode, statusCode(got))
	if reasonContains == "" {
		return
	}

	require.NotNil(t, got)
	assert.Contains(t, got.Message(), reasonContains)
}

func statusCode(status *fwk.Status) fwk.Code {
	if status == nil {
		return fwk.Success
	}
	return status.Code()
}
