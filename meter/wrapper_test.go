package meter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retryMeter fails instantiation on demand
type retryMeter struct {
	mu  sync.Mutex
	err error
}

func (m *retryMeter) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *retryMeter) CurrentPower() (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return 0, m.err
	}
	return 100, nil
}

var retryable = new(retryMeter)

func init() {
	registry.AddCtx("test-retry", func(context.Context, map[string]any) (api.Meter, error) {
		if _, err := retryable.CurrentPower(); err != nil {
			return nil, err
		}
		return retryable, nil
	})
}

func TestWrapperRetry(t *testing.T) {
	retryable.setErr(errors.New("unavailable"))

	_, err := NewFromConfig(context.TODO(), "test-retry", nil)
	require.Error(t, err)

	res := NewWrapper(context.TODO(), "test-retry", nil, err)
	w := res.(*Wrapper)

	// creation just failed: no retry before interval elapsed
	_, err = res.CurrentPower()
	require.ErrorContains(t, err, "meter not available")

	// meter becomes available but retry interval not elapsed: still unavailable
	retryable.setErr(nil)
	_, err = res.CurrentPower()
	require.Error(t, err)
	assert.Nil(t, w.meter)

	// retry interval elapsed: next call creates the meter
	w.mu.Lock()
	w.retriedAt = time.Time{}
	w.mu.Unlock()

	power, err := res.CurrentPower()
	require.NoError(t, err)
	assert.Equal(t, 100.0, power)

	// meter fails at runtime: error passed through, no re-creation
	retryable.setErr(api.ErrTimeout)
	_, err = res.CurrentPower()
	require.ErrorIs(t, err, api.ErrTimeout)
	assert.Same(t, retryable, w.meter)
}
