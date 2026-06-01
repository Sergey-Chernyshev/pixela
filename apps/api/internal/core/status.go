// Package core is the dependency-free domain of Pixela. It imports nothing of ours; every other
// internal package may depend on it, never the reverse. See docs/architecture/go-backend.md §2-§3.
package core

// BuildStatus mirrors the Prisma enum BuildStatus (docs/spec/specs/03-data-model.md).
type BuildStatus string

const (
	BuildRunning        BuildStatus = "RUNNING"         // accepting screenshots
	BuildComparing      BuildStatus = "COMPARING"       // finalized, diffing
	BuildPassed         BuildStatus = "PASSED"          // all unchanged (or all changes approved)
	BuildReviewRequired BuildStatus = "REVIEW_REQUIRED" // has changed/new, needs review
	BuildRejected       BuildStatus = "REJECTED"        // reviewer rejected
	BuildError          BuildStatus = "ERROR"           // processing failure
)

// SnapshotStatus mirrors the Prisma enum SnapshotStatus.
type SnapshotStatus string

const (
	SnapshotPending   SnapshotStatus = "PENDING"
	SnapshotUnchanged SnapshotStatus = "UNCHANGED"
	SnapshotChanged   SnapshotStatus = "CHANGED"
	SnapshotNew       SnapshotStatus = "NEW"
	SnapshotRemoved   SnapshotStatus = "REMOVED"
	SnapshotApproved  SnapshotStatus = "APPROVED"
	SnapshotRejected  SnapshotStatus = "REJECTED"
	SnapshotError     SnapshotStatus = "ERROR"
)

// ApprovalAction mirrors the Prisma enum ApprovalAction.
type ApprovalAction string

const (
	ApprovalApprove ApprovalAction = "APPROVE"
	ApprovalReject  ApprovalAction = "REJECT"
)

// Role mirrors the Prisma enum Role.
type Role string

const (
	RoleOwner  Role = "OWNER"
	RoleMember Role = "MEMBER"
)
