package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func runAPIKey(subCmd string, args []string, name, role string) {
	switch subCmd {
	case "list", "ls":
		resp, body, err := makeRequest(http.MethodGet, "/api/v1/api-keys", nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to list API keys: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			APIKeys []domain.APIKey `json:"api_keys"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing API keys response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n🔑 Flagura Active API Keys & Service Tokens")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		if len(data.APIKeys) == 0 {
			fmt.Println("No active API keys found. Generate one using 'flagura api-key create --name=\"...\"'")
			fmt.Println()
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tROLE\tKEY PREFIX\tCREATED BY\tCREATED\tSTATUS")
		fmt.Fprintln(w, "--\t----\t----\t----------\t----------\t-------\t------")
		for _, k := range data.APIKeys {
			status := "🟢 Active"
			if k.Revoked {
				status = "🔴 Revoked"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				k.ID, k.Name, k.Role, k.KeyPrefix, k.CreatedBy, k.CreatedAt.Format("2006-01-02 15:04"), status)
		}
		_ = w.Flush()
		fmt.Println()

	case "create", "new", "add":
		payload := map[string]interface{}{
			"name": name,
			"role": role,
		}
		resp, body, err := makeRequest(http.MethodPost, "/api/v1/api-keys", payload)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to create API key: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			APIKey  domain.APIKey `json:"api_key"`
			Message string        `json:"message"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n✨ Flagura API Key Generated Successfully!")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		fmt.Printf("ID:         %s\n", data.APIKey.ID)
		fmt.Printf("Name:       %s\n", data.APIKey.Name)
		fmt.Printf("Role:       %s\n", data.APIKey.Role)
		fmt.Printf("Created At: %s\n\n", data.APIKey.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
		fmt.Println("🔑 Secret API Key Token (STORE THIS NOW - IT WILL NOT BE SHOWN AGAIN):")
		fmt.Printf("   \033[32m%s\033[0m\n\n", data.APIKey.Key)
		fmt.Println("Usage Example:")
		fmt.Printf("   export FLAGURA_API_KEY=\"%s\"\n", data.APIKey.Key)
		fmt.Println("   flagura list")
		fmt.Println()

	case "revoke", "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura api-key revoke <key-id>\n")
			os.Exit(1)
		}
		id := args[1]
		path := fmt.Sprintf("/api/v1/api-keys/%s", id)
		resp, body, err := makeRequest(http.MethodDelete, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Revoke failed: %s\n", string(body))
			os.Exit(1)
		}
		fmt.Printf("✅ API Key '%s' has been permanently REVOKED across all edge nodes.\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown api-key subcommand: %s. Use list, create, or revoke.\n", subCmd)
		os.Exit(1)
	}
}
