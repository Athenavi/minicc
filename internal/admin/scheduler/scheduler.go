package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron"
)

// TaskHandler is the function signature for scheduled tasks.
type TaskHandler func(ctx context.Context) error

// CronJob represents a scheduled cron job.
type CronJob struct {
	ID          string
	Name        string
	Schedule    string      // cron expression
	Handler     TaskHandler
	Enabled     bool
	LastRunAt   time.Time
	NextRunAt   time.Time
	LastStatus  string // success/failed
	LastError   string
}

// Scheduler manages all scheduled tasks.
type Scheduler struct {
	cron  *gocron.CronScheduler
	tasks map[string]*CronJob
}

// NewScheduler creates a new task scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:  gocron.NewScheduler(time.UTC),
		tasks: make(map[string]*CronJob),
	}
}

// Register adds a new cron job.
func (s *Scheduler) Register(job *CronJob) error {
	if s.cron == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	
	if !job.Enabled {
		s.tasks[job.ID] = job
		slog.Info("Scheduled task registered (disabled)", "job_id", job.ID)
		return nil
	}
	
	// Parse cron expression and schedule
	_, err := s.cron.Every(job.Schedule).Do(func() {
		s.executeTask(job)
	})
	
	if err != nil {
		return fmt.Errorf("failed to schedule task %s: %w", job.ID, err)
	}
	
	s.tasks[job.ID] = job
	slog.Info("Scheduled task registered", "job_id", job.ID, "schedule", job.Schedule)
	return nil
}

// executeTask runs a task handler with error handling.
func (s *Scheduler) executeTask(job *CronJob) {
	start := time.Now()
	ctx := context.Background()
	
	slog.Info("Executing scheduled task", "job_id", job.ID, "name", job.Name)
	
	err := job.Handler(ctx)
	
	job.LastRunAt = start
	job.LastError = ""
	
	if err != nil {
		job.LastStatus = "failed"
		job.LastError = err.Error()
		slog.Error("Scheduled task failed", "job_id", job.ID, "error", err)
	} else {
		job.LastStatus = "success"
		slog.Info("Scheduled task completed", "job_id", job.ID, "duration_ms", time.Since(start).Milliseconds())
	}
}

// Start begins all scheduled tasks.
func (s *Scheduler) Start() error {
	if s.cron == nil {
		return fmt.Errorf("scheduler not initialized")
	}
	
	s.cron.StartAsync()
	slog.Info("Scheduler started")
	return nil
}

// Stop stops all scheduled tasks.
func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		slog.Info("Scheduler stopped")
	}
}

// ListJobs returns all registered jobs.
func (s *Scheduler) ListJobs() []*CronJob {
	jobs := make([]*CronJob, 0, len(s.tasks))
	for _, job := range s.tasks {
		jobs = append(jobs, job)
	}
	return jobs
}

// RegisterDefaultTasks registers all default system tasks.
func (s *Scheduler) RegisterDefaultTasks() {
	// 1. Cleanup expired API keys (every day at 2:00 AM)
	s.Register(&CronJob{
		ID:       "cleanup_expired_keys",
		Name:     "清理过期 API Key",
		Schedule: "0 2 * * *",
		Handler:  s.cleanupExpiredKeys,
		Enabled:  true,
	})
	
	// 2. Generate weekly usage report (every Sunday at 6:00 AM)
	s.Register(&CronJob{
		ID:       "weekly_usage_report",
		Name:     "生成用量统计报表",
		Schedule: "0 6 * * 0",
		Handler:  s.generateUsageReport,
		Enabled:  true,
	})
	
	// 3. Refresh model cache (every 5 minutes)
	s.Register(&CronJob{
		ID:       "refresh_model_cache",
		Name:     "刷新模型配置缓存",
		Schedule: "*/5 * * * *",
		Handler:  s.refreshModelCache,
		Enabled:  true,
	})
	
	// 4. Health check models (every 30 seconds)
	s.Register(&CronJob{
		ID:       "health_check",
		Name:     "模型健康检查",
		Schedule: "*/30 * * * *",
		Handler:  s.healthCheckModels,
		Enabled:  true,
	})
}

// cleanupExpiredKeys suspends all expired API keys.
func (s *Scheduler) cleanupExpiredKeys(ctx context.Context) error {
	// TODO: Implement actual database query
	slog.Info("Cleaning up expired API keys")
	return nil
}

// generateUsageReport generates a weekly usage statistics report.
func (s *Scheduler) generateUsageReport(ctx context.Context) error {
	// TODO: Query database and generate report
	slog.Info("Generating weekly usage report")
	return nil
}

// refreshModelCache refreshes the model configuration cache.
func (s *Scheduler) refreshModelCache(ctx context.Context) error {
	// TODO: Reload models from database
	slog.Info("Refreshing model cache")
	return nil
}

// healthCheckModels checks the health of all configured models.
func (s *Scheduler) healthCheckModels(ctx context.Context) error {
	// TODO: Ping each model provider
	slog.Info("Running model health check")
	return nil
}
