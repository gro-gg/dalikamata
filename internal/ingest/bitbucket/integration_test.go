//go:build integration

package bitbucket_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/matryer/is"
)

// moduleRoot returns the module root directory. go test sets the working
// directory to the package under test (internal/ingest/bitbucket/).
func moduleRoot() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "../../..")
}

// goRun creates a subprocess that runs `go run . <args>` from the module root.
// Setpgid places the process (and the binary it forks) in their own process
// group so stopSubprocess can kill the whole group, preventing orphaned
// children from keeping I/O pipes open after the test exits.
func goRun(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", append([]string{"run", ".", "--debug"}, args...)...)
	cmd.Dir = moduleRoot()
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	fmt.Println("  goRun:", cmd.String())
	return cmd
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func waitHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("service at %s did not become ready within 30s", url)
}

// scanDomainOutput pipes the domain subprocess stdout through a scanner that
// mirrors every line to os.Stderr and signals two channels:
//   - ready: closed when the NATS consumer is set up ("Repo Handler Settiing Up")
//   - reposDone: closed when 5 "received repo" log lines have been seen
//
// Call before domainSvc.Start(); register domainW.Close() in t.Cleanup AFTER
// stopSubprocess so the scanner goroutine drains cleanly on shutdown.
func scanDomainOutput(domainSvc *exec.Cmd) (domainW *io.PipeWriter, ready, reposDone <-chan struct{}) {
	domainR, w := io.Pipe()
	domainSvc.Stdout = w
	chReady := make(chan struct{})
	chRepos := make(chan struct{})
	go func() {
		readyClosed, reposClosed := false, false
		repoCount := 0
		scanner := bufio.NewScanner(domainR)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)
			if !readyClosed && strings.Contains(line, "Repo Handler Settiing Up") {
				close(chReady)
				readyClosed = true
			}
			if !reposClosed && strings.Contains(line, "subject=ingest.git.repo") {
				repoCount++
				if repoCount >= 5 {
					close(chRepos)
					reposClosed = true
				}
			}
		}
	}()
	return w, chReady, chRepos
}

// stopSubprocess kills the entire process group of cmd (covering both the
// `go run` parent and the compiled binary it forks), then waits for cmd to exit.
func stopSubprocess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	fmt.Println("killpg subprocess:", cmd.Process.Pid)
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Wait()
}

func TestIngestBitbucketIntegration(t *testing.T) {
	t.Parallel()

	is := is.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	bbPort := freePort(t)
	natsPort := freePort(t)

	fmt.Println("1. Start fake Bitbucket first — no dependencies on NATS.")
	fakeBB := goRun(ctx, "fake", "bitbucket",
		"--addr", fmt.Sprintf("127.0.0.1:%d", bbPort))
	is.NoErr(fakeBB.Start())
	t.Cleanup(func() { stopSubprocess(fakeBB) })

	waitHTTP(t,
		fmt.Sprintf("http://127.0.0.1:%d/rest/api/1.0/projects/PROJ/repos", bbPort))

	fmt.Println("2. Start domain service (embedded NATS + domain port).")
	natsDataDir := t.TempDir()
	domainSvc := goRun(ctx, "domain",
		"--nats-port", strconv.Itoa(natsPort),
		"--nats-data", natsDataDir)
	domainW, domainReady, reposDone := scanDomainOutput(domainSvc)
	is.NoErr(domainSvc.Start())
	t.Cleanup(func() { stopSubprocess(domainSvc); domainW.Close() })

	fmt.Println("waiting for domain service to be ready (log: \"Repo Handler Settiing Up\")...")
	select {
	case <-domainReady:
	case <-ctx.Done():
		t.Fatal("timed out waiting for domain service to be ready")
	}

	fmt.Println("3. Start ingest service — crawls fake Bitbucket and publishes to NATS.")
	ingestSvc := goRun(ctx, "ingest", "bitbucket",
		//"--nats-port", strconv.Itoa(natsPort), // TODO bug: domain starts nats always on 4222
		"--bitbucket-url", fmt.Sprintf("http://127.0.0.1:%d", bbPort),
		"--bitbucket-token", "test-token",
		"--bitbucket-projects", "PROJ,INFRA")
	is.NoErr(ingestSvc.Start())
	t.Cleanup(func() { stopSubprocess(ingestSvc) })

	fmt.Println("4. Wait for domain to log 5 received repo events (subject=ingest.git.repo).")
	select {
	case <-reposDone:
	case <-ctx.Done():
		t.Fatal("timed out waiting for domain to receive all 5 repo events")
	}
	fmt.Println("Test completed successfully.")
}
