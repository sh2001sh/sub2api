package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/cpaconvert"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type cpaImportResponse struct {
	Conversion cpaconvert.Summary `json:"conversion"`
	Import     DataImportResult   `json:"import"`

	PreservedAPIKeys []cpaconvert.PreservedAPIKey `json:"preserved_api_keys"`
	SkippedAccounts  []cpaconvert.SkippedAccount  `json:"skipped_accounts"`
	Warnings         []string                     `json:"warnings"`
}

type cpaImportIdempotencyPayload struct {
	Files                []cpaImportIdempotencyFile `json:"files"`
	SkipDefaultGroupBind bool                       `json:"skip_default_group_bind"`
}

type cpaImportIdempotencyFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ImportCPA converts uploaded CPA auth/config files into original sub2api import
// data and immediately reuses the existing account import pipeline.
func (h *AccountHandler) ImportCPA(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		response.BadRequest(c, "Invalid multipart form: "+err.Error())
		return
	}

	files := collectUploadedCPAFileHeaders(form)
	if len(files) == 0 {
		response.BadRequest(c, "No CPA files uploaded")
		return
	}

	skipDefaultGroupBind := true
	if raw := strings.TrimSpace(c.PostForm("skip_default_group_bind")); raw != "" {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			response.BadRequest(c, "Invalid skip_default_group_bind: "+parseErr.Error())
			return
		}
		skipDefaultGroupBind = parsed
	}

	tempRoot, err := os.MkdirTemp("", "sub2api-cpa-import-*")
	if err != nil {
		response.InternalError(c, "Failed to create temp dir: "+err.Error())
		return
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	idempotencyFiles, err := persistUploadedCPAFiles(tempRoot, files)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(idempotencyFiles) == 0 {
		response.BadRequest(c, "No supported CPA files found; upload auth JSON files and optional config/config.yaml")
		return
	}

	sort.Slice(idempotencyFiles, func(i, j int) bool {
		return idempotencyFiles[i].Path < idempotencyFiles[j].Path
	})

	payload := cpaImportIdempotencyPayload{
		Files:                idempotencyFiles,
		SkipDefaultGroupBind: skipDefaultGroupBind,
	}

	result, execErr := executeAdminIdempotent(c, "admin.accounts.import_cpa", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		converted, convertErr := cpaconvert.ConvertDir(tempRoot)
		if convertErr != nil {
			return nil, convertErr
		}

		importResult, importErr := h.importData(ctx, DataImportRequest{
			Data:                 converted.DataPayload,
			SkipDefaultGroupBind: &skipDefaultGroupBind,
		})
		if importErr != nil {
			return nil, importErr
		}

		return cpaImportResponse{
			Conversion:       converted.Summary,
			Import:           importResult,
			PreservedAPIKeys: converted.PreservedAPIKeys,
			SkippedAccounts:  converted.SkippedAccounts,
			Warnings:         converted.Warnings,
		}, nil
	})
	if execErr != nil {
		response.ErrorFrom(c, execErr)
		return
	}
	if result != nil && result.Replayed {
		c.Header("X-Idempotency-Replayed", "true")
	}
	response.Success(c, result.Data)
}

func collectUploadedCPAFileHeaders(form *multipart.Form) []*multipart.FileHeader {
	if form == nil || len(form.File) == 0 {
		return nil
	}

	files := make([]*multipart.FileHeader, 0)
	for _, items := range form.File {
		files = append(files, items...)
	}
	return files
}

func persistUploadedCPAFiles(rootDir string, files []*multipart.FileHeader) ([]cpaImportIdempotencyFile, error) {
	out := make([]cpaImportIdempotencyFile, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}

		relativePath, ok := normalizeUploadedCPAPath(file.Filename)
		if !ok {
			continue
		}

		source, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open uploaded file %s: %w", file.Filename, err)
		}

		targetPath := filepath.Join(rootDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			_ = source.Close()
			return nil, fmt.Errorf("prepare temp file %s: %w", relativePath, err)
		}

		target, err := os.Create(targetPath)
		if err != nil {
			_ = source.Close()
			return nil, fmt.Errorf("create temp file %s: %w", relativePath, err)
		}

		hasher := sha256.New()
		writer := io.MultiWriter(target, hasher)
		_, copyErr := io.Copy(writer, source)
		closeErr := source.Close()
		targetCloseErr := target.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("write temp file %s: %w", relativePath, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close uploaded file %s: %w", file.Filename, closeErr)
		}
		if targetCloseErr != nil {
			return nil, fmt.Errorf("close temp file %s: %w", relativePath, targetCloseErr)
		}

		out = append(out, cpaImportIdempotencyFile{
			Path:   relativePath,
			Size:   file.Size,
			SHA256: hex.EncodeToString(hasher.Sum(nil)),
		})
	}
	return out, nil
}

func normalizeUploadedCPAPath(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}

	value = filepath.ToSlash(value)
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	segments := strings.Split(value, "/")
	if len(segments) == 0 {
		return "", false
	}

	baseName := strings.TrimSpace(segments[len(segments)-1])
	if baseName == "" {
		return "", false
	}
	lowerBase := strings.ToLower(baseName)

	if lowerBase == "config.yaml" || lowerBase == "config.yml" {
		return "config/config.yaml", true
	}

	if strings.EqualFold(filepath.Ext(baseName), ".json") {
		if len(segments) == 1 {
			return "auths/" + baseName, true
		}
		for _, segment := range segments[:len(segments)-1] {
			normalized := strings.ToLower(strings.TrimSpace(segment))
			if normalized == "auths" {
				return "auths/" + baseName, true
			}
		}
		return "", false
	}

	return "", false
}
