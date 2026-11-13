package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "norix",
		Short: "Norix control-plane CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			token, _ := cmd.Root().PersistentFlags().GetString("token")
			if strings.TrimSpace(token) == "" {
				if stored, err := readTokenFile(); err == nil && stored != "" {
					_ = cmd.Root().PersistentFlags().Set("token", stored)
				}
			}
			return nil
		},
	}
	root.PersistentFlags().String("api", envOr("NORIX_API", "http://127.0.0.1:8080"), "control plane base URL")
	root.PersistentFlags().String("token", os.Getenv("NORIX_TOKEN"), "session bearer token")

	root.AddCommand(loginCmd(), projectsCmd(), deployCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store API token for subsequent commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, _ := cmd.Flags().GetString("token")
			if strings.TrimSpace(token) == "" {
				token, _ = cmd.Root().PersistentFlags().GetString("token")
			}
			if strings.TrimSpace(token) == "" {
				token = strings.TrimSpace(os.Getenv("NORIX_TOKEN"))
			}
			if token == "" {
				return fmt.Errorf("set NORIX_TOKEN, pass --token, or obtain a session token from the web UI")
			}
			path, err := writeTokenFile(token)
			if err != nil {
				return err
			}
			fmt.Printf("Saved token to %s\n", path)
			return nil
		},
	}
	cmd.Flags().String("token", "", "session token (defaults to NORIX_TOKEN)")
	cmd.Flags().String("email", "", "account email (reserved)")
	cmd.Flags().String("password", "", "account password (reserved)")
	return cmd
}

func projectsCmd() *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiBase, _ := cmd.Root().PersistentFlags().GetString("api")
			token, _ := cmd.Root().PersistentFlags().GetString("token")
			if strings.TrimSpace(token) == "" {
				return fmt.Errorf("authenticate with `norix login` or set NORIX_TOKEN")
			}
			body, err := connectJSON(cmd.Context(), apiBase, token, "/v1.ProjectService/ListProjects", map[string]any{})
			if err != nil {
				return err
			}
			fmt.Println(string(body))
			return nil
		},
	}
	projects := &cobra.Command{Use: "projects", Short: "Project commands"}
	projects.AddCommand(list)
	return projects
}

func deployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a workload (stub)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, _ := cmd.Flags().GetString("project")
			name, _ := cmd.Flags().GetString("name")
			image, _ := cmd.Flags().GetString("image")
			fmt.Printf("TODO: deploy workload project=%s name=%s image=%s\n", projectID, name, image)
			fmt.Println("Wire to WorkloadService/Deploy once stable; flags are accepted for scripting.")
			return nil
		},
	}
	cmd.Flags().String("project", "", "project id")
	cmd.Flags().String("name", "", "workload name")
	cmd.Flags().String("image", "", "container image")
	return cmd
}

func norixConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".norix"), nil
}

func tokenFilePath() (string, error) {
	dir, err := norixConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token"), nil
}

func writeTokenFile(token string) (string, error) {
	dir, err := norixConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func readTokenFile() (string, error) {
	path, err := tokenFilePath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func connectJSON(ctx context.Context, apiBase, token, procedure string, payload map[string]any) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	url := strings.TrimRight(apiBase, "/") + "/api" + procedure
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API %s: HTTP %d: %s", procedure, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
