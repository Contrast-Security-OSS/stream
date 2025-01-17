package stream

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Scaler implements generic auto-scaling logic which starts with a net-zero
// set of processing routines (with the exception of the channel listener) and
// then scales up and down based on the CPU contention of a system and the speed
// at which the InterceptionFunc is able to process data. Once the incoming
// channel becomes blocked (due to nothing being sent) each of the spawned
// routines will finish out their execution of Fn and then the internal timer
// will collapse brining the routine count back to zero until there is more to
// be done.
//
// To use Scalar, simply create a new Scaler[T, U], configuring the Wait, Life,
// and InterceptFunc fields. These fields are what configure the functionality
// of the Scaler.
//
// NOTE: Fn is REQUIRED!
//
// After creating the Scaler instance and configuring it, call the Exec method
// passing the appropriate context and input channel.
//
// Internally the Scaler implementation will wait for data on the incoming
// channel and attempt to send it to a layer2 channel. If the layer2 channel
// is blocking and the Wait time has been reached, then the Scaler will spawn
// a new layer2 which will increase throughput for the Scaler, and Scaler
// will attempt to send the data to the layer2 channel once more. This process
// will repeat until a successful send occurs. (This should only loop twice)
type Scaler[T, U any] struct {
	Wait time.Duration
	Life time.Duration
	Fn   InterceptFunc[T, U]
}

// Exec starts the internal Scaler routine (the first layer of processing) and
// returns the output channel where the resulting data from the Fn function
// will be sent.
func (s Scaler[T, U]) Exec(ctx context.Context, in <-chan T) (<-chan U, error) {
	ctx = _ctx(ctx)

	// Fn is REQUIRED!
	if s.Fn == nil {
		return nil, fmt.Errorf("invalid <nil> InterceptFunc")
	}

	// Create outbound channel
	out := make(chan U)

	// nano-second precision really isn't feasible here, so this is arbitrary
	// because the caller did not specify a wait time. This means Scaler will
	// likely always scale up rather than waiting for an existing layer2 routine
	// to pick up data.
	if s.Wait <= 0 {
		s.Wait = time.Nanosecond
	}

	// Minimum life of a spawned layer2 should be 1ms
	if s.Life < time.Microsecond {
		s.Life = time.Microsecond
	}

	go func() {
		defer recover()
		defer close(out)

		wg := sync.WaitGroup{}
		wgMu := sync.Mutex{}

		// Ensure that the method does not close
		// until all layer2 routines have exited
		defer func() {
			wgMu.Lock()
			wg.Wait()
			wgMu.Unlock()
		}()

		l2 := make(chan T)
		ticker := time.NewTicker(s.Wait)
		defer ticker.Stop()

	scaleLoop:
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					break scaleLoop
				}

			l2loop:
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						wgMu.Lock()
						wg.Add(1)
						wgMu.Unlock()

						go func() {
							defer wg.Done()

							Pipe(ctx, s.layer2(ctx, l2), out)
						}()
					case l2 <- v:
						break l2loop
					}
				}

				// Reset the ticker so that it does not immediately trip the
				// case statement on loop.
				ticker.Reset(s.Wait)
			}
		}
	}()

	return out, nil
}

// layer2 manages the execution of the InterceptFunc. layer2 has a life time
// of s.Life and will exit if the context is canceled, the timer has reached
// its life time, or the incoming channel has been closed.
//
// If the case statement which reads from the in channel is executed, then
// layer2 will execute the Scaler function and send the result to the out
// channel. Afterward, layer2 will reset the internal timer, expanding the
// life time of the layer2, and continue to attempt another read from the in
// channel until the in channel is closed, the context is canceled, or the
// timer has reached its life time.
func (s Scaler[T, U]) layer2(ctx context.Context, in <-chan T) <-chan U {
	out := make(chan U)

	go func() {
		defer recover()
		defer close(out)

		timer := time.NewTimer(s.Life)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				return
			case t, ok := <-in:
				if !ok {
					return
				}

				// If the function returns false, then don't send the data
				// but break out of the select statement to ensure the timer
				// is reset.
				u, send := s.Fn(ctx, t)
				if !send {
					break
				}

				// Send the resulting value to the output channel
				select {
				case <-ctx.Done():
					return
				case out <- u:
				}
			}

			// NOTE: This code is based off the doc comment for time.Timer.Stop
			// which ensures that the channel of the timer is drained before
			// resetting the timer so that it doesn't immediately trip the
			// case statement.
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(s.Life)
		}
	}()

	return out
}
