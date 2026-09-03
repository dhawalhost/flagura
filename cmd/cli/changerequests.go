package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func runChangeRequest(subCmd string, args []string, status, comments string) {
	switch subCmd {
	case "list", "ls":
		path := "/api/v1/change-requests"
		if status != "" {
			path += "?status=" + status
		}
		resp, body, err := makeRequest(http.MethodGet, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Failed to list change requests: %s\n", string(body))
			os.Exit(1)
		}
		if jsonOut {
			fmt.Println(string(body))
			return
		}

		var data struct {
			ChangeRequests []domain.ChangeRequest `json:"change_requests"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing change requests: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n🛡️  Flagura 4-Eyes Governance Change Requests")
		fmt.Println("─────────────────────────────────────────────")
		if len(data.ChangeRequests) == 0 {
			fmt.Println("No change requests found.")
			fmt.Println()
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tFLAG KEY\tENV\tAUTHOR\tSTATUS\tCREATED")
		fmt.Fprintln(w, "--\t--------\t---\t------\t------\t-------")
		for _, cr := range data.ChangeRequests {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				cr.ID, cr.FlagKey, cr.Environment, cr.AuthorEmail, cr.Status, cr.CreatedAt.Format("2006-01-02 15:04"))
		}
		_ = w.Flush()
		fmt.Println()

	case "approve", "reject":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura change-request %s <id> [--comments=\"...\"]\n", subCmd)
			os.Exit(1)
		}
		id := args[1]
		approved := subCmd == "approve"
		path := fmt.Sprintf("/api/v1/change-requests/%s/review", id)
		payload := map[string]interface{}{
			"approved": approved,
			"comments": comments,
		}
		resp, body, err := makeRequest(http.MethodPost, path, payload)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Review failed: %s\n", string(body))
			os.Exit(1)
		}
		if approved {
			fmt.Printf("✅ ChangeRequest '%s' has been APPROVED.\n", id)
		} else {
			fmt.Printf("❌ ChangeRequest '%s' has been REJECTED.\n", id)
		}

	case "apply":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: flagura change-request apply <id>\n")
			os.Exit(1)
		}
		id := args[1]
		path := fmt.Sprintf("/api/v1/change-requests/%s/apply", id)
		resp, body, err := makeRequest(http.MethodPost, path, nil)
		if err != nil || resp.StatusCode >= 400 {
			fmt.Fprintf(os.Stderr, "Apply failed: %s\n", string(body))
			os.Exit(1)
		}
		fmt.Printf("🚀 ChangeRequest '%s' successfully APPLIED to live feature flag config!\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown change-request subcommand: %s. Use list, approve, reject, or apply.\n", subCmd)
		os.Exit(1)
	}
}
