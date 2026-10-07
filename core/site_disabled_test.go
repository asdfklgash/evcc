package core

import (
	"testing"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/metrics"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSiteUpdateDisabledDevices disables all devices while running (nil instance, see updateDevice).
// The update loop must skip them instead of polling them.
func TestSiteUpdateDisabledDevices(t *testing.T) {
	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	require.NoError(t, metrics.SetupSchema())
	config.Reset()

	disabled := func(name string) config.Device[api.Meter] {
		return config.NewStaticDevice(config.Named{Name: name}, api.Meter(nil))
	}

	site := NewSite()
	site.log = util.NewLogger("foo")
	site.valueChan = make(chan util.Param, 1024)

	site.gridMeter = disabled("grid")
	site.pvMeters = []config.Device[api.Meter]{disabled("pv")}
	site.batteryMeters = []config.Device[api.Meter]{disabled("battery")}
	site.auxMeters = []config.Device[api.Meter]{disabled("aux")}
	site.extMeters = []config.Device[api.Meter]{disabled("ext")}
	site.consumerMeters = []config.Device[api.Meter]{disabled("consumer")}
	site.curtailers = []config.Device[api.Curtailer]{config.NewStaticDevice(config.Named{Name: "curtailer"}, api.Curtailer(nil))}
	require.NoError(t, config.Circuits().Add(config.NewStaticDevice(config.Named{Name: "circuit"}, api.Circuit(nil))))

	for name, group := range map[string]string{
		"grid": metrics.Grid, "pv": metrics.PV, "battery": metrics.Battery,
		"aux": metrics.Consumer, "consumer": metrics.Consumer, "ext": metrics.Meter,
	} {
		col, err := metrics.NewCollector(group, name, name)
		require.NoError(t, err)
		site.collectors[name] = col
	}

	require.NotPanics(t, func() {
		require.Error(t, site.updateGridMeter(), "no control with stale grid power")
		site.updatePvMeters()
		site.updateBatteryMeters()
		site.updateAuxMeters()
		site.updateExtMeters()
		site.updateConsumerMeters()
		site.publishCircuits()
		assert.Empty(t, site.curtailables())
	})

	assert.Zero(t, site.pvPower)
	assert.Zero(t, site.auxPower)
}
