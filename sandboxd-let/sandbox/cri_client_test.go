package sandbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// fakeImageService implements runtimeapi.ImageServiceClient for tests. Only
// ImageStatus and PullImage are exercised; the embedded nil interface
// supplies the rest (they would panic if ever called, which they are not).
type fakeImageService struct {
	runtimeapi.ImageServiceClient

	mu sync.Mutex
	// present marks images already in the local store (ImageStatus hits).
	present map[string]bool
	// per-ref count of how many times PullImage actually executed.
	pullCounts map[string]int
	// inFlight / maxInFlight track concurrent PullImage executions.
	inFlight    int
	maxInFlight int

	pullDelay  time.Duration
	statusErr  error
	pullErr    error
	pullErrRef string // if set, only this ref's pull fails
}

func newFakeImageService(delay time.Duration) *fakeImageService {
	return &fakeImageService{
		present:    map[string]bool{},
		pullCounts: map[string]int{},
		pullDelay:  delay,
	}
}

func (f *fakeImageService) totalPulls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.pullCounts {
		n += c
	}
	return n
}

func (f *fakeImageService) pullsFor(ref string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pullCounts[ref]
}

func (f *fakeImageService) maxConcurrent() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *fakeImageService) ImageStatus(_ context.Context, in *runtimeapi.ImageStatusRequest, _ ...grpc.CallOption) (*runtimeapi.ImageStatusResponse, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	ref := in.GetImage().GetImage()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.present[ref] {
		return &runtimeapi.ImageStatusResponse{Image: &runtimeapi.Image{Id: ref}}, nil
	}
	return &runtimeapi.ImageStatusResponse{}, nil
}

func (f *fakeImageService) PullImage(ctx context.Context, in *runtimeapi.PullImageRequest, _ ...grpc.CallOption) (*runtimeapi.PullImageResponse, error) {
	ref := in.GetImage().GetImage()

	f.mu.Lock()
	f.pullCounts[ref]++
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	select {
	case <-time.After(f.pullDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if f.pullErr != nil && (f.pullErrRef == "" || f.pullErrRef == ref) {
		return nil, f.pullErr
	}

	f.mu.Lock()
	f.present[ref] = true
	f.mu.Unlock()
	return &runtimeapi.PullImageResponse{ImageRef: ref}, nil
}

// pullN fires n concurrent pullImage calls for the given ref and returns the
// per-call errors once all complete.
func pullN(c *criClient, ref string, n int, callerTimeout time.Duration) []error {
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all goroutines together to maximize overlap
			ctx, cancel := context.WithTimeout(context.Background(), callerTimeout)
			defer cancel()
			errs[idx] = c.pullImage(ctx, ref)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func TestPullImage_DedupSameRef(t *testing.T) {
	fake := newFakeImageService(300 * time.Millisecond)
	c := &criClient{image: fake}

	const n = 25
	errs := pullN(c, "registry.example.com/app:latest", n, 10*time.Second)

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d returned error: %v", i, err)
		}
	}
	if got := fake.pullsFor("registry.example.com/app:latest"); got != 1 {
		t.Fatalf("expected exactly 1 PullImage for %d concurrent identical pulls, got %d", n, got)
	}
	if got := fake.maxConcurrent(); got != 1 {
		t.Fatalf("expected max concurrent PullImage of 1, got %d", got)
	}
}

func TestPullImage_DistinctRefsRunInParallel(t *testing.T) {
	fake := newFakeImageService(200 * time.Millisecond)
	c := &criClient{image: fake}

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ref := "registry.example.com/app:v" + string(rune('a'+idx))
			if err := c.pullImage(ctx, ref); err != nil {
				t.Errorf("pull %d: %v", idx, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if got := fake.totalPulls(); got != n {
		t.Fatalf("expected %d PullImage calls for %d distinct images, got %d", n, n, got)
	}
	// Different refs must NOT be serialized: they overlap.
	if got := fake.maxConcurrent(); got < 2 {
		t.Fatalf("expected distinct-image pulls to run concurrently (max>=2), got max=%d", got)
	}
}

func TestPullImage_CachedSkipsPull(t *testing.T) {
	fake := newFakeImageService(0)
	fake.present["registry.example.com/app:cached"] = true
	c := &criClient{image: fake}

	if err := c.pullImage(context.Background(), "registry.example.com/app:cached"); err != nil {
		t.Fatalf("cached pull returned error: %v", err)
	}
	if got := fake.totalPulls(); got != 0 {
		t.Fatalf("expected 0 PullImage for a cached image, got %d", got)
	}
}

func TestPullImage_OneCallerCancelDoesNotAbortOthers(t *testing.T) {
	fake := newFakeImageService(400 * time.Millisecond)
	c := &criClient{image: fake}

	const n = 10
	var wg sync.WaitGroup
	results := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			// Caller 0 gives up early; the rest wait comfortably.
			d := 10 * time.Second
			if idx == 0 {
				d = 50 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), d)
			defer cancel()
			results[idx] = c.pullImage(ctx, "registry.example.com/app:shared")
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != nil {
			t.Fatalf("non-cancelled caller %d failed: %v", i, results[i])
		}
	}
	// The early-cancelled caller may or may not have already attached; if it
	// returned an error it must be a context error, not a pull failure.
	if results[0] != nil && !errors.Is(results[0], context.DeadlineExceeded) {
		t.Fatalf("cancelled caller got unexpected error: %v", results[0])
	}
	if got := fake.pullsFor("registry.example.com/app:shared"); got != 1 {
		t.Fatalf("expected exactly 1 PullImage, got %d", got)
	}
}

func TestPullImage_SharedErrorPropagatesToAllWaiters(t *testing.T) {
	fake := newFakeImageService(150 * time.Millisecond)
	fake.pullErr = errors.New("boom: registry unauthorized")
	c := &criClient{image: fake}

	const n = 12
	errs := pullN(c, "registry.example.com/app:bad", n, 10*time.Second)

	for i, err := range errs {
		if err == nil {
			t.Fatalf("caller %d expected the shared pull error, got nil", i)
		}
	}
	// The failed pull ran once; the image must not be marked present.
	if got := fake.pullsFor("registry.example.com/app:bad"); got != 1 {
		t.Fatalf("expected exactly 1 (failed) PullImage, got %d", got)
	}
}

func TestPullImage_RepullAfterInFlightCompletes(t *testing.T) {
	fake := newFakeImageService(80 * time.Millisecond)
	c := &criClient{image: fake}
	ref := "registry.example.com/app:seq"

	// First batch: concurrent, deduped to one pull.
	for _, err := range pullN(c, ref, 5, 10*time.Second) {
		if err != nil {
			t.Fatalf("first batch error: %v", err)
		}
	}
	if got := fake.pullsFor(ref); got != 1 {
		t.Fatalf("after first batch expected 1 pull, got %d", got)
	}

	// Second call after the first completed: image is now cached, so it must
	// take the fast path and not pull again (singleflight does not cache
	// results across completed calls; caching does).
	if err := c.pullImage(context.Background(), ref); err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if got := fake.pullsFor(ref); got != 1 {
		t.Fatalf("expected no additional pull after caching, got %d total", got)
	}
}

func TestPullImage_UsesCallerDeadlineForSharedPull(t *testing.T) {
	// Pull is slower than the caller's deadline -> the shared pull is bounded
	// by that deadline and fails with a context error rather than hanging.
	fake := newFakeImageService(2 * time.Second)
	c := &criClient{image: fake}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	err := c.pullImage(ctx, "registry.example.com/app:slow")
	elapsed := time.Since(startedAt)

	if err == nil {
		t.Fatalf("expected a timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Fatalf("pull should have given up near the caller deadline, took %s", elapsed)
	}
}
