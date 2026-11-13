package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func OpsBackupsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		switch request.Method {
		case http.MethodGet:
			items, listErr := db.ListBackups(request.Context(), pool, identity.OrganizationID)
			if listErr != nil {
				http.Error(response, listErr.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(response, map[string]any{"backups": items})
		case http.MethodPost:
			backup, createErr := db.CreatePostgresBackup(request.Context(), pool, identity.OrganizationID, identity.Email)
			if createErr != nil && backup.ID == "" {
				http.Error(response, createErr.Error(), http.StatusBadRequest)
				return
			}
			status := http.StatusCreated
			if createErr != nil {
				status = http.StatusBadRequest
			}
			response.WriteHeader(status)
			writeJSON(response, map[string]any{"backup": backup, "error": errorString(createErr)})
		default:
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func OpsRestoreHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		backupID := strings.TrimSpace(request.PathValue("backupID"))
		backup, restoreErr := db.RestorePostgresBackup(request.Context(), pool, identity.OrganizationID, backupID)
		if restoreErr != nil && backup.ID == "" {
			http.Error(response, restoreErr.Error(), http.StatusBadRequest)
			return
		}
		status := http.StatusOK
		if restoreErr != nil {
			status = http.StatusBadRequest
		}
		response.WriteHeader(status)
		writeJSON(response, map[string]any{"backup": backup, "error": errorString(restoreErr)})
	}
}

func OpsBackupDownloadHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		backupID := strings.TrimSpace(request.PathValue("backupID"))
		backup, err := db.GetBackup(request.Context(), pool, identity.OrganizationID, backupID)
		if err != nil {
			http.Error(response, err.Error(), http.StatusNotFound)
			return
		}
		if backup.Status == "failed" || backup.Filename == "" {
			http.Error(response, "backup file is not available", http.StatusBadRequest)
			return
		}
		path := filepath.Join(db.BackupDir(), backup.Filename)
		if _, statErr := os.Stat(path); statErr != nil {
			http.Error(response, "backup file missing on disk", http.StatusNotFound)
			return
		}
		response.Header().Set("Content-Type", "application/sql")
		response.Header().Set("Content-Disposition", `attachment; filename="`+backup.Filename+`"`)
		http.ServeFile(response, request, path)
	}
}

func OpsAlertsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		items, listErr := db.ListAlerts(request.Context(), pool, identity.OrganizationID, 50)
		if listErr != nil {
			http.Error(response, listErr.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(response, map[string]any{"alerts": items})
	}
}

func sessionFromRequest(request *http.Request, pool *pgxpool.Pool) (auth.SessionIdentity, error) {
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return auth.SessionIdentity{}, errUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return findActiveSession(request.Context(), pool, auth.HashSessionToken(token))
}

var errUnauthorized = &httpError{message: "Bearer token required"}

type httpError struct{ message string }

func (err *httpError) Error() string { return err.message }

func writeJSON(response http.ResponseWriter, payload any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(payload)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
