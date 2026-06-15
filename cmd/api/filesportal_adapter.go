package main

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/pkg/filesportal"
)

// filesPortalAdapter adapts *filesportal.Client to
// service.QRISDocPortalUploader so the registration service can upload
// onboarding documents without importing the portal package directly.
type filesPortalAdapter struct {
	client     *filesportal.Client
	accessMode string // open | once
}

func (a *filesPortalAdapter) Upload(ctx context.Context, businessName string, docs []service.QRISDocRequest) (string, string, error) {
	files := make([]filesportal.UploadFile, 0, len(docs))
	for _, d := range docs {
		files = append(files, filesportal.UploadFile{
			FileName:   d.FileName,
			DataBase64: d.Content,
		})
	}

	req := filesportal.UploadRequest{
		Title:      "QRIS Onboarding — " + businessName,
		DocName:    businessName,
		AccessMode: a.accessMode,
		Files:      files,
	}
	if strings.EqualFold(a.accessMode, "once") {
		req.OnceNote = "Dokumen sensitif sekali-unduh. Segera unduh lalu konfirmasi."
	}

	resp, err := a.client.Upload(ctx, req)
	if err != nil {
		return "", "", err
	}
	return resp.BundleURL, resp.Token, nil
}
