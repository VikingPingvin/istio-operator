package versions

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/maistra/istio-operator/pkg/apis/maistra/v1"
	"github.com/maistra/istio-operator/pkg/controller/common"
	"github.com/maistra/istio-operator/pkg/controller/common/test/assert"
)

func TestV2_1ChartOrder(t *testing.T) {
	//check the predefined order only, as changes might affect fundamental behaviour
	expectedOrder := [][]string{
		{DiscoveryChart},
		{MeshConfigChart},
		{TelemetryCommonChart, PrometheusChart},
		{RemoteChart},
		{MixerPolicyChart, MixerTelemetryChart, TracingChart, GatewayIngressChart, GatewayEgressChart, GrafanaChart},
		{KialiChart},
		{ThreeScaleChart, WASMExtensionsChart},
	}
	for i := 0; i < len(v2_1ChartOrder); i++ {
		if !cmp.Equal(v2_1ChartOrder[i], expectedOrder[i]) {
			t.Errorf("Incorrect chart order definition at element: %s", v2_1ChartOrder[i])
		}
	}
}

func TestV2_1_checkAndSetupRemoteDataPlaneConfig_OK(t *testing.T) {
	ctx := context.TODO()
	log := common.LogFromContext(ctx)
	v2_1 := &versionStrategyV2_1{version: V2_1}
	v2_1ChartMapping = map[string]chartRenderingDetails{
		DiscoveryChart: {
			path:         "istio-control/istio-discovery",
			enabledField: "",
		},
		TelemetryCommonChart: {
			path:         "istio-telemetry/telemetry-common",
			enabledField: "",
		},
		MeshConfigChart: {
			path:         "mesh-config",
			enabledField: "",
		},
		RemoteChart: {
			path:         "istiod-remote",
			enabledField: "remote",
		},
	}

	istioHelm := v1.NewHelmValues(map[string]interface{}{
		"remote": map[string]interface{}{
			"enabled": true,
		},
	},
	)

	assert.True(v2_1ChartMapping[DiscoveryChart].enabledField != "noway", "Dsicovery chart init value problem", t)
	assert.True(v2_1ChartMapping[TelemetryCommonChart].enabledField != "noway", "Telemetry chart init value problem", t)
	assert.True(v2_1ChartMapping[MeshConfigChart].enabledField != "noway", "MeshConfig chart init value problem", t)
	v2_1.checkAndSetupRemoteDataPlaneConfig(istioHelm, log)
	assert.Equals(v2_1ChartMapping[DiscoveryChart].enabledField, "noway", "Discovery chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[TelemetryCommonChart].enabledField, "noway", "Telemetry chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[MeshConfigChart].enabledField, "noway", "MeshConfig chart is not disabled", t)
}

func TestV2_1_checkAndSetupRemoteDataPlaneConfig_Skip(t *testing.T) {
	//no Remote chart case
	ctx := context.TODO()
	log := common.LogFromContext(ctx)
	v2_1 := &versionStrategyV2_1{version: V2_1}
	v2_1ChartMapping = map[string]chartRenderingDetails{
		DiscoveryChart: {
			path:         "istio-control/istio-discovery",
			enabledField: "",
		},
	}

	istioHelm := v1.NewHelmValues(map[string]interface{}{
		"remote": map[string]interface{}{
			"enabled": true,
		},
	})

	assert.True(v2_1ChartMapping[DiscoveryChart].enabledField != "noway", "Dsicovery chart init value problem", t)
	assert.True(v2_1ChartMapping[TelemetryCommonChart].enabledField != "noway", "Telemetry chart init value problem", t)
	assert.True(v2_1ChartMapping[MeshConfigChart].enabledField != "noway", "MeshConfig chart init value problem", t)
	v2_1.checkAndSetupRemoteDataPlaneConfig(istioHelm, log)
	assert.Equals(v2_1ChartMapping[DiscoveryChart].enabledField, "", "Discovery chart handling is modified", t)
	assert.Equals(v2_1ChartMapping[TelemetryCommonChart].enabledField, "", "Telemetry chart handling is modified", t)
	assert.Equals(v2_1ChartMapping[MeshConfigChart].enabledField, "", "MeshConfig chart handling is modified", t)

	//no helm chart real values
	assert.True(v2_1ChartMapping[DiscoveryChart].enabledField != "noway", "Dsicovery chart init value problem", t)
	assert.True(v2_1ChartMapping[TelemetryCommonChart].enabledField != "noway", "Telemetry chart init value problem", t)
	assert.True(v2_1ChartMapping[MeshConfigChart].enabledField != "noway", "MeshConfig chart init value problem", t)
	v2_1.checkAndSetupRemoteDataPlaneConfig(nil, log)
	assert.Equals(v2_1ChartMapping[DiscoveryChart].enabledField, "", "Discovery chart handling is modified", t)
	assert.Equals(v2_1ChartMapping[TelemetryCommonChart].enabledField, "", "Telemetry chart handling is modified", t)
	assert.Equals(v2_1ChartMapping[MeshConfigChart].enabledField, "", "MeshConfig chart handling is modified", t)

}

func TestV2_1_checkAndSetupExternalControlPlaneConfig(t *testing.T) {
	ctx := context.TODO()
	log := common.LogFromContext(ctx)
	v2_1 := &versionStrategyV2_1{version: V2_1}
	v2_1ChartMapping = map[string]chartRenderingDetails{
		GatewayIngressChart: {
			path:         "gateways/istio-ingress",
			enabledField: "",
		},
		GatewayEgressChart: {
			path:         "gateways/istio-egress",
			enabledField: "",
		},
		TelemetryCommonChart: {
			path:         "istio-telemetry/telemetry-common",
			enabledField: "",
		},
		MeshConfigChart: {
			path:         "mesh-config",
			enabledField: "",
		},
	}

	externalProfile := false
	//external profile was not found
	assert.True(v2_1ChartMapping[GatewayEgressChart].enabledField != "noway", "GwEgress chart init value problem", t)
	assert.True(v2_1ChartMapping[GatewayIngressChart].enabledField != "noway", "GwIngress chart init value problem", t)
	assert.True(v2_1ChartMapping[TelemetryCommonChart].enabledField != "noway", "Telemetry chart init value problem", t)
	assert.True(v2_1ChartMapping[MeshConfigChart].enabledField != "noway", "MeshConfig chart init value problem", t)
	v2_1.checkAndSetupExternalControlPlaneConfig(externalProfile, log)
	assert.Equals(v2_1ChartMapping[GatewayEgressChart].enabledField, "", "GwEgress chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[GatewayIngressChart].enabledField, "", "GwIngress chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[TelemetryCommonChart].enabledField, "", "Telemetry chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[MeshConfigChart].enabledField, "", "MeshConfig chart is not disabled", t)

	externalProfile = true
	//external profile was found
	assert.True(v2_1ChartMapping[GatewayEgressChart].enabledField != "noway", "GwEgress chart init value problem", t)
	assert.True(v2_1ChartMapping[GatewayIngressChart].enabledField != "noway", "GwIngress chart init value problem", t)
	assert.True(v2_1ChartMapping[TelemetryCommonChart].enabledField != "noway", "Telemetry chart init value problem", t)
	assert.True(v2_1ChartMapping[MeshConfigChart].enabledField != "noway", "MeshConfig chart init value problem", t)
	v2_1.checkAndSetupExternalControlPlaneConfig(externalProfile, log)
	assert.Equals(v2_1ChartMapping[GatewayEgressChart].enabledField, "noway", "GwEgress chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[GatewayIngressChart].enabledField, "noway", "GwIngress chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[TelemetryCommonChart].enabledField, "noway", "Telemetry chart is not disabled", t)
	assert.Equals(v2_1ChartMapping[MeshConfigChart].enabledField, "noway", "MeshConfig chart is not disabled", t)

}

func TestV2_1_isExternalProfileActive(t *testing.T) {
	v2_1 := &versionStrategyV2_1{version: V2_1}
	profiles := []string{"q", "external"}
	verdict := v2_1.isExternalProfileActive(profiles)
	assert.True(verdict, "External profile flag should be set", t)

	profiles = []string{"q"}
	verdict = v2_1.isExternalProfileActive(profiles)
	assert.False(verdict, "External profile flag should NOT be set", t)
}
