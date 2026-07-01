package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	internalauth "github.com/tm-lbenson/nexus-local/services/api/internal/auth"
	"github.com/tm-lbenson/nexus-local/services/api/internal/config"
	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

const shutdownConfirmation = "SHUTDOWN"

type runtimeShutdownRequest struct {
	TenantID     string `json:"tenant_id"`
	Confirmation string `json:"confirmation"`
}

type dockerContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
}

func runtimeShutdownHandler(cfg config.Config, authorizer internalauth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req runtimeShutdownRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if strings.TrimSpace(req.Confirmation) != shutdownConfirmation {
			writeError(w, http.StatusBadRequest, "type SHUTDOWN to confirm")
			return
		}
		if _, ok := requireTenantPermission(w, r, authorizer, domain.TenantID(req.TenantID), domain.PermissionManageTenant); !ok {
			return
		}
		if !cfg.UIShutdownEnabled {
			writeError(w, http.StatusForbidden, "UI shutdown is disabled for this deployment")
			return
		}
		if !runtimeShutdownAvailable(cfg) {
			writeError(w, http.StatusServiceUnavailable, "Docker control socket is not available")
			return
		}

		client := newDockerSocketClient(cfg.UIShutdownDockerSocket)
		project := shutdownComposeProject(cfg)
		containers, err := listProjectContainers(r.Context(), client, project)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("list containers: %v", err))
			return
		}
		running := runningProjectContainers(containers)
		if len(running) == 0 {
			writeJSON(w, http.StatusConflict, envelope{
				"status":  "idle",
				"message": "No running Nexus Local containers were found.",
				"project": project,
			})
			return
		}

		writeJSON(w, http.StatusAccepted, envelope{
			"status":     "scheduled",
			"message":    "Nexus Local shutdown has been scheduled.",
			"project":    project,
			"containers": encodeShutdownContainers(running),
		})
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		go func() {
			time.Sleep(750 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), cfg.UIShutdownStopTimeout+30*time.Second)
			defer cancel()
			if err := stopProjectContainers(ctx, client, running, cfg.UIShutdownStopTimeout); err != nil {
				log.Printf("runtime shutdown failed: %v", err)
			}
		}()
	}
}

func runtimeShutdownAvailable(cfg config.Config) bool {
	if !cfg.UIShutdownEnabled {
		return false
	}
	socketPath := shutdownDockerSocket(cfg)
	if socketPath == "" {
		return false
	}
	info, err := os.Stat(socketPath)
	return err == nil && !info.IsDir()
}

func shutdownDockerSocket(cfg config.Config) string {
	socketPath := strings.TrimSpace(cfg.UIShutdownDockerSocket)
	if socketPath == "" {
		return "/var/run/docker.sock"
	}
	return socketPath
}

func shutdownComposeProject(cfg config.Config) string {
	project := strings.TrimSpace(cfg.UIShutdownComposeProject)
	if project == "" {
		return "nexus-local"
	}
	return project
}

func newDockerSocketClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", shutdownDockerSocket(config.Config{UIShutdownDockerSocket: socketPath}))
		},
	}
	return &http.Client{Transport: transport}
}

func listProjectContainers(ctx context.Context, client *http.Client, project string) ([]dockerContainerSummary, error) {
	filters, err := json.Marshal(map[string][]string{
		"label": {"com.docker.compose.project=" + project},
	})
	if err != nil {
		return nil, err
	}
	endpoint := "http://docker/containers/json?all=1&filters=" + url.QueryEscape(string(filters))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("docker returned %s", resp.Status)
	}
	var containers []dockerContainerSummary
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func runningProjectContainers(containers []dockerContainerSummary) []dockerContainerSummary {
	running := make([]dockerContainerSummary, 0, len(containers))
	for _, container := range containers {
		switch strings.ToLower(container.State) {
		case "running", "restarting", "paused":
			running = append(running, container)
		}
	}
	sort.SliceStable(running, func(i, j int) bool {
		return shutdownStopOrder(running[i]) < shutdownStopOrder(running[j])
	})
	return running
}

func shutdownStopOrder(container dockerContainerSummary) int {
	switch container.Labels["com.docker.compose.service"] {
	case "api":
		return 100
	case "web":
		return 80
	case "worker":
		return 10
	default:
		return 50
	}
}

func stopProjectContainers(ctx context.Context, client *http.Client, containers []dockerContainerSummary, timeout time.Duration) error {
	timeoutSeconds := int(timeout.Seconds())
	if timeoutSeconds < 1 {
		timeoutSeconds = 10
	}
	for _, container := range containers {
		containerID := strings.TrimSpace(container.ID)
		if containerID == "" {
			continue
		}
		endpoint := fmt.Sprintf("http://docker/containers/%s/stop?t=%d", url.PathEscape(containerID), timeoutSeconds)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
			return fmt.Errorf("stop %s: docker returned %s", shutdownContainerName(container), resp.Status)
		}
		log.Printf("stopped Nexus Local container %s", shutdownContainerName(container))
	}
	return nil
}

func encodeShutdownContainers(containers []dockerContainerSummary) []envelope {
	encoded := make([]envelope, 0, len(containers))
	for _, container := range containers {
		encoded = append(encoded, envelope{
			"id":      shortContainerID(container.ID),
			"name":    shutdownContainerName(container),
			"service": container.Labels["com.docker.compose.service"],
			"state":   container.State,
		})
	}
	return encoded
}

func shutdownContainerName(container dockerContainerSummary) string {
	if len(container.Names) > 0 {
		return strings.TrimPrefix(container.Names[0], "/")
	}
	return shortContainerID(container.ID)
}

func shortContainerID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
