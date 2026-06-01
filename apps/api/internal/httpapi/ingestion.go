package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/ingestion"
)

// apiKeySecurity is the per-operation security requirement that turns on the ApiKey guard.
var apiKeySecurity = []map[string][]string{{apiKeySchemeName: {}}}

// registerIngestion wires the CI ingestion endpoints (API contract §"Endpoints приёма") onto the Huma
// API. Every operation declares apiKeySecurity, so the middleware enforces the key and the project.
func registerIngestion(api huma.API, svc *ingestion.Service, log *slog.Logger) {
	huma.Register(api, huma.Operation{
		OperationID: "createBuild", Method: http.MethodPost, Path: "/v1/builds",
		Summary: "Create a build", Tags: []string{"ingestion"},
		DefaultStatus: http.StatusCreated, Security: apiKeySecurity,
	}, func(ctx context.Context, in *createBuildInput) (*createBuildOutput, error) {
		p, err := requirePrincipal(ctx)
		if err != nil {
			return nil, err
		}
		build, err := svc.CreateBuild(ctx, p.ProjectID, ingestion.CreateBuildInput{
			Branch:    in.Body.Branch,
			CommitSha: in.Body.CommitSha,
			CIBuildID: in.Body.CIBuildID,
			CIJobURL:  in.Body.CIJobURL,
			MRIID:     in.Body.MRIID,
		})
		if err != nil {
			return nil, mapError(log, err)
		}
		out := &createBuildOutput{}
		out.Body.BuildID = build.ID
		out.Body.Status = string(build.Status)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "declareSnapshot", Method: http.MethodPost, Path: "/v1/builds/{buildId}/snapshots",
		Summary: "Declare a screenshot by hash (step 1 of 2)", Tags: []string{"ingestion"},
		Security: apiKeySecurity,
	}, func(ctx context.Context, in *declareSnapshotInput) (*declareSnapshotOutput, error) {
		p, err := requirePrincipal(ctx)
		if err != nil {
			return nil, err
		}
		snapshotID, needUpload, err := svc.DeclareSnapshot(ctx, p.ProjectID, in.BuildID, ingestion.DeclareSnapshotInput{
			Name:         in.Body.Name,
			Browser:      in.Body.Browser,
			Viewport:     in.Body.Viewport,
			ImageSha256:  in.Body.ImageSha256,
			Width:        in.Body.Width,
			Height:       in.Body.Height,
			ByteSize:     in.Body.ByteSize,
			BaselinePath: in.Body.BaselinePath,
		})
		if err != nil {
			return nil, mapError(log, err)
		}
		out := &declareSnapshotOutput{}
		out.Body.SnapshotID = snapshotID
		out.Body.NeedUpload = needUpload
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "uploadImage", Method: http.MethodPut, Path: "/v1/images/{sha256}",
		Summary: "Upload screenshot bytes (step 2 of 2)", Tags: []string{"ingestion"},
		DefaultStatus: http.StatusNoContent, Security: apiKeySecurity,
	}, func(ctx context.Context, in *uploadImageInput) (*struct{}, error) {
		if _, err := requirePrincipal(ctx); err != nil {
			return nil, err
		}
		if err := svc.UploadImage(ctx, in.Sha256, in.RawBody); err != nil {
			return nil, mapError(log, err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "finalizeBuild", Method: http.MethodPatch, Path: "/v1/builds/{buildId}",
		Summary: "Finalize a build (compute REMOVED, enqueue diff)", Tags: []string{"ingestion"},
		Security: apiKeySecurity,
	}, func(ctx context.Context, in *finalizeBuildInput) (*createBuildOutput, error) {
		p, err := requirePrincipal(ctx)
		if err != nil {
			return nil, err
		}
		build, err := svc.FinalizeBuild(ctx, p.ProjectID, in.BuildID)
		if err != nil {
			return nil, mapError(log, err)
		}
		out := &createBuildOutput{}
		out.Body.BuildID = build.ID
		out.Body.Status = string(build.Status)
		return out, nil
	})
}

// principal returns the authenticated principal or a 401 (the middleware guarantees it on secured ops;
// this is defense in depth).
func requirePrincipal(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		return auth.Principal{}, newAPIError(http.StatusUnauthorized, core.CodeUnauthorized, "Authentication required")
	}
	return p, nil
}

// ---- DTOs (Huma reflects these into OpenAPI 3.1 + validates inputs at the edge) ----

type createBuildInput struct {
	Body struct {
		Branch        string  `json:"branch" minLength:"1" doc:"Git branch under test"`
		CommitSha     string  `json:"commitSha" minLength:"1"`
		CIBuildID     *string `json:"ciBuildId,omitempty"`
		CIJobURL      *string `json:"ciJobUrl,omitempty"`
		MRIID         *string `json:"mrIid,omitempty"`
		ParallelTotal *int    `json:"parallelTotal,omitempty" doc:"Reserved for shard aggregation (Phase 3)"`
	}
}

type createBuildOutput struct {
	Body struct {
		BuildID string `json:"buildId"`
		Status  string `json:"status"`
	}
}

type declareSnapshotInput struct {
	BuildID string `path:"buildId"`
	Body    struct {
		Name         string  `json:"name" minLength:"1"`
		Browser      string  `json:"browser" minLength:"1"`
		Viewport     string  `json:"viewport" minLength:"1"`
		ImageSha256  string  `json:"imageSha256" pattern:"^[a-f0-9]{64}$" doc:"sha256 of the PNG bytes, lowercase hex"`
		Width        int32   `json:"width" minimum:"1"`
		Height       int32   `json:"height" minimum:"1"`
		ByteSize     int32   `json:"byteSize" minimum:"1"`
		BaselinePath *string `json:"baselinePath,omitempty" doc:"Repo-relative path of the baseline file (Mode A); optional"`
	}
}

type declareSnapshotOutput struct {
	Body struct {
		SnapshotID string `json:"snapshotId"`
		NeedUpload bool   `json:"needUpload"`
	}
}

type uploadImageInput struct {
	Sha256  string `path:"sha256" pattern:"^[a-f0-9]{64}$"`
	RawBody []byte `contentType:"image/png"`
}

type finalizeBuildInput struct {
	BuildID string `path:"buildId"`
	Body    struct {
		Status string `json:"status" enum:"FINALIZE"`
	}
}
