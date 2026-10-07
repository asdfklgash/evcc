package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/site"
	"github.com/evcc-io/evcc/db"
	"github.com/evcc-io/evcc/util/config"
	"github.com/evcc-io/evcc/util/templates"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deleteCircuitTestSite struct {
	site.API
	gridMeterRef string
}

func (s deleteCircuitTestSite) GetGridMeterRef() string {
	return s.gridMeterRef
}

func (s deleteCircuitTestSite) GetPVMeterRefs() []string       { return nil }
func (s deleteCircuitTestSite) GetBatteryMeterRefs() []string  { return nil }
func (s deleteCircuitTestSite) GetAuxMeterRefs() []string      { return nil }
func (s deleteCircuitTestSite) GetExtMeterRefs() []string      { return nil }
func (s deleteCircuitTestSite) GetConsumerMeterRefs() []string { return nil }

func TestDeleteCircuitMeter(t *testing.T) {
	tests := []struct {
		name         string
		gridMeterRef bool
		wantMeter    bool
	}{
		{
			name:      "dedicated meter",
			wantMeter: false,
		},
		{
			name:         "grid meter",
			gridMeterRef: true,
			wantMeter:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, db.NewInstance("sqlite", ":memory:"))
			config.Reset()

			meterConfig, err := config.AddConfig(templates.Meter, map[string]any{"type": "custom"})
			require.NoError(t, err)
			var meterInstance api.Meter
			require.NoError(t, config.Meters().Add(config.NewConfigurableDevice(&meterConfig, meterInstance)))

			circuitConfig, err := config.AddConfig(templates.Circuit, map[string]any{
				"meter": config.NameForID(meterConfig.ID),
			})
			require.NoError(t, err)
			var circuitInstance api.Circuit
			require.NoError(t, config.Circuits().Add(config.NewConfigurableDevice(&circuitConfig, circuitInstance)))

			gridMeterRef := ""
			if tt.gridMeterRef {
				gridMeterRef = config.NameForID(meterConfig.ID)
			}
			testSite := deleteCircuitTestSite{gridMeterRef: gridMeterRef}
			req := httptest.NewRequest(http.MethodDelete, "/", nil)
			req = mux.SetURLVars(req, map[string]string{"class": "circuit", "id": strconv.Itoa(circuitConfig.ID)})
			rec := httptest.NewRecorder()

			deleteDeviceHandler(testSite)(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			_, err = config.Circuits().ByName(config.NameForID(circuitConfig.ID))
			assert.Error(t, err)
			_, err = config.Meters().ByName(config.NameForID(meterConfig.ID))
			if tt.wantMeter {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestDeviceConfigMapCustomYamlRedacted(t *testing.T) {
	const yamlStr = "type: template\ntemplate: foo\nmaxPower: 2760\npassword: secret123\n"

	dev := config.NewConfigurableDevice[api.Charger](&config.Config{
		ID:         1,
		Class:      templates.Charger,
		Properties: config.Properties{Type: "custom"},
		Data:       map[string]any{"yaml": yamlStr, "password": "secret123"},
	}, nil)

	for hidePrivate, want := range map[bool]string{false: "secret123", true: "*****"} {
		dc, err := deviceConfigMap(templates.Charger, dev, hidePrivate)
		require.NoError(t, err)

		conf := dc["config"].(map[string]any)
		assert.Contains(t, conf["yaml"], "password: "+want)
		assert.Contains(t, conf["yaml"], "maxPower: 2760")
		assert.Equal(t, want, conf["password"])
	}
}

type updateDeviceTestMeter struct{}

func (updateDeviceTestMeter) CurrentPower() (float64, error) { return 0, nil }

// TestUpdateDeviceKeepsInstance asserts that a device in use is never replaced by a nil instance
func TestUpdateDeviceKeepsInstance(t *testing.T) {
	other := map[string]any{"template": "demo-meter", "usage": "charge", "power": 100}

	tests := []struct {
		name    string
		disable bool
		force   bool
		wantErr bool
	}{
		{name: "update fails", wantErr: true},
		{name: "force update", force: true},
		{name: "disable", disable: true, force: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, db.NewInstance("sqlite", ":memory:"))
			config.Reset()

			conf, err := config.AddConfig(templates.Meter, other, config.WithProperties(config.Properties{Type: "template"}))
			require.NoError(t, err)

			running := new(updateDeviceTestMeter)
			require.NoError(t, config.Meters().Add(config.NewConfigurableDevice(&conf, api.Meter(running))))

			var created int
			unavailable := func(context.Context, string, map[string]any) (api.Meter, error) {
				created++
				return nil, errors.New("unavailable")
			}

			req := configReq{Properties: config.Properties{Type: "template", Disable: tt.disable}, Other: other}
			err = updateDevice(context.TODO(), conf.ID, templates.Meter, req, unavailable, config.Meters(), tt.force)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			dev, err := config.Meters().ByName(config.NameForID(conf.ID))
			require.NoError(t, err)
			assert.Same(t, running, dev.Instance())
			assert.Equal(t, !tt.disable, created > 0, "device created")

			stored, err := config.ConfigByID(conf.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.disable, stored.Disable)
		})
	}
}
