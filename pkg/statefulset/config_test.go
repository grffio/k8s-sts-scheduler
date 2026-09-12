package statefulset_test

import (
	"context"
	"testing"

	statefulset "github.com/grffio/k8s-sts-scheduler/pkg/statefulset"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fwk "k8s.io/kube-scheduler/framework"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      runtime.Object
		errContains string
		wantErr     bool
	}{
		{
			name: "valid JSON config",
			config: &runtime.Unknown{
				Raw: []byte(`{"nodeOrdinalLabel":"topology.example.com/ordinal"}`),
			},
		},
		{
			name: "valid YAML config",
			config: &runtime.Unknown{
				Raw:         []byte("nodeOrdinalLabel: topology.example.com/ordinal\n"),
				ContentType: runtime.ContentTypeYAML,
			},
		},
		{
			name:        "nil config",
			config:      nil,
			errContains: "nodeOrdinalLabel is required",
			wantErr:     true,
		},
		{
			name: "malformed JSON",
			config: &runtime.Unknown{
				Raw: []byte(`{"nodeOrdinalLabel":`),
			},
			errContains: "decode configuration",
			wantErr:     true,
		},
		{
			name:        "unexpected runtime object",
			config:      &corev1.Pod{},
			errContains: "decode configuration",
			wantErr:     true,
		},
		{
			name: "unsupported content type",
			config: &runtime.Unknown{
				Raw:         []byte(`{"nodeOrdinalLabel":"ordinal"}`),
				ContentType: "application/toml",
			},
			errContains: "decode configuration",
			wantErr:     true,
		},
		{
			name: "missing nodeOrdinalLabel",
			config: &runtime.Unknown{
				Raw: []byte(`{}`),
			},
			errContains: "nodeOrdinalLabel is required",
			wantErr:     true,
		},
		{
			name: "invalid Kubernetes label key",
			config: &runtime.Unknown{
				Raw: []byte(`{"nodeOrdinalLabel":"example.com/foo/bar"}`),
			},
			errContains: "is not a valid Kubernetes label key",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := statefulset.New(context.Background(), tt.config, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, statefulset.Name, got.Name())

			_, ok := got.(fwk.SignPlugin)
			assert.True(t, ok)
			_, ok = got.(fwk.PreFilterPlugin)
			assert.True(t, ok)
			_, ok = got.(fwk.FilterPlugin)
			assert.True(t, ok)
			_, ok = got.(fwk.EnqueueExtensions)
			assert.True(t, ok)
		})
	}
}

func TestNew_NodeOrdinalLabelValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		label       string
		errContains string
		wantErr     bool
	}{
		{
			name:  "simple label name",
			label: "ordinal",
		},
		{
			name:  "qualified label name",
			label: "topology.example.com/ordinal",
		},
		{
			name:        "empty label name",
			label:       "",
			errContains: "nodeOrdinalLabel is required",
			wantErr:     true,
		},
		{
			name:        "multiple slashes",
			label:       "example.com/foo/bar",
			errContains: "is not a valid Kubernetes label key",
			wantErr:     true,
		},
		{
			name:        "invalid DNS prefix",
			label:       "Example.COM/ordinal",
			errContains: "is not a valid Kubernetes label key",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := statefulset.New(
				context.Background(),
				runtimeConfig(t, statefulset.Args{NodeOrdinalLabel: tt.label}),
				nil,
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

func TestNew_ConfiguresNodeOrdinalLabel(t *testing.T) {
	t.Parallel()

	const configuredLabel = "scheduler.example.com/statefulset-ordinal"

	contracts := newPlugin(t, configuredLabel)
	pod := statefulSetPod("default", "db-7", "7")
	state := newCycleState()

	_, status := contracts.preFilter.PreFilter(context.Background(), state, pod, nil)
	assertStatus(t, status, fwk.Success, "")

	status = contracts.filter.Filter(
		context.Background(),
		state,
		pod,
		nodeInfo(node("worker-7", map[string]string{configuredLabel: "7"})),
	)
	assertStatus(t, status, fwk.Success, "")

	status = contracts.filter.Filter(
		context.Background(),
		state,
		pod,
		nodeInfo(node("worker-default-key", map[string]string{testNodeOrdinalLabel: "7"})),
	)
	assertStatus(t, status, fwk.UnschedulableAndUnresolvable, "missing required ordinal label")
}

func TestPluginMetadata(t *testing.T) {
	t.Parallel()

	contracts := newPlugin(t, testNodeOrdinalLabel)

	assert.Equal(t, statefulset.Name, contracts.plugin.Name())
	assert.Nil(t, contracts.preFilter.PreFilterExtensions())
}
