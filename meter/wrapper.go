package meter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/util"
)

// retryInterval is the minimum time between attempts to create an unavailable meter
const retryInterval = 5 * time.Minute

// Wrapper wraps an api.Meter that could not be created and retries creating it.
// Only power is exposed, the meter's other capabilities are unknown until it has been created.
type Wrapper struct {
	mu     sync.Mutex
	log    *util.Logger
	ctx    context.Context
	typ    string
	config map[string]any

	meter     api.Meter
	err       error
	retriedAt time.Time // last creation attempt
}

var _ api.Meter = (*Wrapper)(nil)

// NewWrapper creates a wrapper for a meter that could not be created
func NewWrapper(ctx context.Context, typ string, other map[string]any, err error) api.Meter {
	return &Wrapper{
		log:       util.ContextLoggerWithDefault(ctx, util.NewLogger("meter")),
		ctx:       ctx,
		typ:       typ,
		config:    other,
		err:       err,
		retriedAt: time.Now(),
	}
}

// WrappedConfig indicates a device with wrapped configuration
func (v *Wrapper) WrappedConfig() (string, map[string]any) {
	return v.typ, v.config
}

// instance returns the meter once created. Creation is retried at most once per retryInterval.
func (v *Wrapper) instance() api.Meter {
	if v.meter != nil || time.Since(v.retriedAt) < retryInterval {
		return v.meter
	}

	v.retriedAt = time.Now()

	m, err := NewFromConfig(v.ctx, v.typ, v.config)
	if err != nil {
		v.err = err
		v.log.WARN.Printf("creating meter failed: %v", err)
		return nil
	}

	v.meter = m
	v.log.INFO.Printf("meter available: %s", util.TypeWithTemplateName(v.typ, v.config))

	return m
}

// CurrentPower implements the api.Meter interface
func (v *Wrapper) CurrentPower() (float64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if m := v.instance(); m != nil {
		return m.CurrentPower()
	}

	return 0, fmt.Errorf("meter not available: %w", v.err)
}
