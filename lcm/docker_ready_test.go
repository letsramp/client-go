/*
 * Copyright Skyramp Authors 2026
 */

package lcm

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	dockerTypes "github.com/docker/docker/api/types"
)

func containerWithHealth(status string) dockerTypes.ContainerJSON {
	c := dockerTypes.ContainerJSON{ContainerJSONBase: &dockerTypes.ContainerJSONBase{State: &dockerTypes.ContainerState{Running: true}}}
	if status != "" {
		c.State.Health = &dockerTypes.Health{Status: status}
	}
	return c
}

func readyzServer(t *testing.T, code int) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return n
}

// TestIsContainerReadyPrefersTheHealthcheck: an image that declares one is judged by it
// and nothing else.
func TestIsContainerReadyPrefersTheHealthcheck(t *testing.T) {
	if !isContainerReady(containerWithHealth("healthy"), "", 0) {
		t.Fatal("healthy must be ready without any probe")
	}
	if isContainerReady(containerWithHealth("starting"), "", readyzServer(t, http.StatusOK)) {
		t.Fatal("a declared healthcheck that is still starting wins over a 200 from /readyz")
	}
}

// TestIsContainerReadyFallsBackToReadyz: no healthcheck (the lean Windows worker) means
// the worker's own /readyz decides, so the poll loop can end.
func TestIsContainerReadyFallsBackToReadyz(t *testing.T) {
	if !isContainerReady(containerWithHealth(""), "", readyzServer(t, http.StatusOK)) {
		t.Fatal("no healthcheck + /readyz 200 must be ready")
	}
	if isContainerReady(containerWithHealth(""), "", readyzServer(t, http.StatusServiceUnavailable)) {
		t.Fatal("no healthcheck + /readyz 503 must not be ready")
	}
	if isContainerReady(containerWithHealth(""), "", 0) {
		t.Fatal("no healthcheck and no port to probe must not be ready")
	}
}

// TestReadinessHostFollowsTheDaemon: this package builds its client with docker.FromEnv,
// so DOCKER_HOST can name another machine. The worker's published port is then on that
// machine, and probing localhost asks a host that is not running the container.
func TestReadinessHostFollowsTheDaemon(t *testing.T) {
	for _, tc := range []struct {
		daemon string
		want   string
	}{
		{"", "127.0.0.1"},
		{"unix:///var/run/docker.sock", "127.0.0.1"},
		{"npipe:////./pipe/docker_engine", "127.0.0.1"},
		{"tcp://engine.internal:2376", "engine.internal"},
		{"https://engine.internal:2376", "engine.internal"},
		{":::not a url", "127.0.0.1"},
	} {
		if got := readinessHost(tc.daemon); got != tc.want {
			t.Errorf("readinessHost(%q) = %q, want %q", tc.daemon, got, tc.want)
		}
	}
}
