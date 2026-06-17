package agent

import (
	"time"

	"gorm.io/gorm"
)

// Fix job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusPROpen  = "pr_open"
	StatusFailed  = "failed"
)

// FixJob is one unit of agent work: read an issue, open a pull request that
// fixes it. Jobs move queued -> running -> pr_open, or land in failed.
type FixJob struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	APIKeyID  uint      `gorm:"index" json:"api_key_id"`         // which app
	IssueHash string    `gorm:"index;size:64" json:"issue_hash"` // the lognorth issue this fixes
	Issue     string    `gorm:"size:500" json:"issue"`           // human-readable issue title or ref
	Status    string    `gorm:"size:20;index" json:"status"`
	PRURL     string    `gorm:"size:500" json:"pr_url"`
	Detail    string    `gorm:"size:1000" json:"detail"` // failure reason or note
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name.
func (FixJob) TableName() string {
	return "fix_jobs"
}

// AutoMigrate creates the fix_jobs table.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&FixJob{})
}

// ListFixJobs returns jobs newest-first. apiKeyID 0 returns every app's jobs.
func ListFixJobs(db *gorm.DB, apiKeyID uint) ([]FixJob, error) {
	var jobs []FixJob
	q := db.Order("updated_at desc")
	if apiKeyID != 0 {
		q = q.Where("api_key_id = ?", apiKeyID)
	}
	err := q.Find(&jobs).Error
	return jobs, err
}

// FindFixJobForIssue returns the latest fix job for one issue, or nil if none.
func FindFixJobForIssue(db *gorm.DB, apiKeyID uint, issueHash string) (*FixJob, error) {
	var job FixJob
	err := db.Where("api_key_id = ? AND issue_hash = ?", apiKeyID, issueHash).
		Order("updated_at desc").First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}
