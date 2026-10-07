package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/meter"
	meterconfig "github.com/evcc-io/evcc/meter/config"
	"github.com/evcc-io/evcc/util/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	meterconfig.Registry.AddCtx("test-unavailable", func(context.Context, map[string]any) (api.Meter, error) {
		return nil, errors.New("unavailable")
	})
}

// TestConfigureMetersUnavailableConsumer asserts that an unavailable consumer meter does not fail boot
func TestConfigureMetersUnavailableConsumer(t *testing.T) {
	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	config.Reset()

	references.consumerMeter = []string{"consumer"}
	t.Cleanup(func() { references.consumerMeter = nil })

	static := []config.Named{{Name: "consumer", Type: "test-unavailable"}}
	require.NoError(t, configureMeters(static, "consumer"))

	dev, err := config.Meters().ByName("consumer")
	require.NoError(t, err)
	assert.IsType(t, new(meter.Wrapper), dev.Instance())

	_, err = dev.Instance().CurrentPower()
	require.ErrorContains(t, err, "meter not available")
}

// TestConfigureMetersUnavailableGrid asserts that other unavailable meters still fail boot
func TestConfigureMetersUnavailableGrid(t *testing.T) {
	require.NoError(t, db.NewInstance("sqlite", ":memory:"))
	config.Reset()

	references.consumerMeter = []string{"consumer"}
	t.Cleanup(func() { references.consumerMeter = nil })

	static := []config.Named{{Name: "grid", Type: "test-unavailable"}}
	require.Error(t, configureMeters(static, "grid"))
}
