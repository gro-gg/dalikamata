//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	dalinats "codeberg.org/aeforged/dalikamata/internal/domain/nats"
)

func moduleRoot() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "../..")
}

func TestMain(m *testing.M) {
	root := moduleRoot()
	for _, img := range []struct{ tag, dockerfile string }{
		{"dalikamata:latest", "deploy/docker/Dockerfile"},
		{"dalifakes:latest", "deploy/docker/Dockerfile.fakes"},
	} {
		cmd := exec.Command("docker", "build", "-t", img.tag, "-f", img.dockerfile, ".")
		cmd.Dir = root
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("building image %s: %v", img.tag, err)
		}
	}
	os.Exit(m.Run())
}

func composeFile(name string) string {
	return filepath.Join(moduleRoot(), "deploy", "docker", name)
}

func TestE2EMono(t *testing.T) {
	t.Parallel()
	runE2E(t,
		"dalikamata-e2e-mono",
		composeFile("docker-compose-e2e-mono.yaml"),
		12112,
		14222,
	)
}

func TestE2EMicro(t *testing.T) {
	t.Parallel()
	runE2E(t,
		"dalikamata-e2e-micro",
		composeFile("docker-compose-e2e-micro.yaml"),
		22112,
		24222,
	)
}

func runE2E(t *testing.T, project, file string, metricsPort, natsPort int) {
	t.Helper()

	dockerComposeUp(t, project, file)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	metricsURL := fmt.Sprintf("http://localhost:%d/metrics", metricsPort)
	waitForMetric(t, ctx, metricsURL, "pr_cycle_time_seconds_count")

	natsURL := fmt.Sprintf("nats://localhost:%d", natsPort)
	checkNATSStream(t, ctx, natsURL)
}

// dockerComposeUp runs `docker compose up -d --build` and registers cleanup to
// bring the stack down. It uses a named project to isolate parallel test runs.
func dockerComposeUp(t *testing.T, project, file string) {
	t.Helper()

	run(t, "docker", "compose", "-p", project, "-f", file, "up", "-d")

	t.Cleanup(func() {
		run(t, "docker", "compose", "-p", project, "-f", file, "down", "--volumes", "--remove-orphans")
	})
}

// waitForMetric polls metricsURL until it finds a line containing metricName
// with a non-zero value, or ctx is cancelled.
func waitForMetric(t *testing.T, ctx context.Context, metricsURL, metricName string) {
	t.Helper()
	for {
		body, err := fetchMetrics(metricsURL)
		if err == nil && hasNonZeroMetric(body, metricName) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for metric %s at %s", metricName, metricsURL)
		case <-time.After(2 * time.Second):
		}
	}
}

// checkNATSStream connects to natsURL and verifies the INGEST stream exists
// and contains at least one message.
func checkNATSStream(t *testing.T, ctx context.Context, natsURL string) {
	t.Helper()
	var js jetstream.JetStream
	for {
		nc, err := nats.Connect(natsURL)
		if err == nil {
			js, err = jetstream.New(nc)
			if err == nil {
				t.Cleanup(nc.Close)
				break
			}
			nc.Close()
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out connecting to NATS at %s", natsURL)
		case <-time.After(2 * time.Second):
		}
	}

	for {
		stream, err := js.Stream(ctx, dalinats.StreamIngestName)
		if err == nil {
			info, err := stream.Info(ctx)
			if err == nil && info.State.Msgs > 0 {
				t.Logf("NATS stream %s has %d messages", dalinats.StreamIngestName, info.State.Msgs)
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for messages in NATS stream %s", dalinats.StreamIngestName)
		case <-time.After(2 * time.Second):
		}
	}
}

func fetchMetrics(url string) (string, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// hasNonZeroMetric returns true if body contains a line for metricName whose
// value is greater than zero.
func hasNonZeroMetric(body, metricName string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") || !strings.Contains(line, metricName) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err == nil && val > 0 {
			return true
		}
	}
	return false
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("command %q failed: %v", strings.Join(append([]string{name}, args...), " "), err)
	}
}
