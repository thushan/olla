package health

import (
	"errors"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type WorkerPool struct {
	workerCount   int
	jobCh         chan healthCheckJob
	stopCh        chan struct{}
	wg            sync.WaitGroup
	healthClient  *HealthClient
	repository    domain.EndpointRepository
	statusTracker *StatusTransitionTracker
	logger        *logger.StyledLogger
}

func NewWorkerPool(
	workerCount int,
	queueSize int,
	healthClient *HealthClient,
	repository domain.EndpointRepository,
	statusTracker *StatusTransitionTracker,
	logger *logger.StyledLogger,
) *WorkerPool {
	jobCh := make(chan healthCheckJob, queueSize)

	return &WorkerPool{
		workerCount:   workerCount,
		jobCh:         jobCh,
		stopCh:        make(chan struct{}),
		healthClient:  healthClient,
		repository:    repository,
		statusTracker: statusTracker,
		logger:        logger,
	}
}

func (wp *WorkerPool) Start(scheduler *HealthScheduler) {
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(scheduler)
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.stopCh)
	wp.wg.Wait()
}

func (wp *WorkerPool) GetJobChannel() chan<- healthCheckJob {
	return wp.jobCh
}

func (wp *WorkerPool) GetQueueStats() (int, int, float64) {
	queueSize := len(wp.jobCh)
	queueCap := cap(wp.jobCh)
	queueUsage := float64(queueSize) / float64(queueCap)
	return queueSize, queueCap, queueUsage
}

func (wp *WorkerPool) worker(scheduler *HealthScheduler) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.stopCh:
			return
		case job := <-wp.jobCh:
			wp.processHealthCheck(job, scheduler)
		}
	}
}

func (wp *WorkerPool) processHealthCheck(job healthCheckJob, scheduler *HealthScheduler) {
	result, err := wp.healthClient.Check(job.ctx, job.endpoint)

	job.endpoint.Status = result.Status
	job.endpoint.LastChecked = time.Now()
	job.endpoint.LastLatency = result.Latency

	isSuccess := result.Status == domain.StatusHealthy
	nextInterval, newMultiplier := calculateBackoff(job.endpoint, isSuccess)

	if !isSuccess {
		job.endpoint.ConsecutiveFailures++
		job.endpoint.BackoffMultiplier = newMultiplier
	} else {
		job.endpoint.ConsecutiveFailures = 0
		job.endpoint.BackoffMultiplier = 1
	}

	job.endpoint.NextCheckTime = time.Now().Add(nextInterval)

	// Check if endpoint still exists before updating
	if !wp.repository.Exists(job.ctx, job.endpoint.URL) {
		wp.logger.Debug("Endpoint removed from configuration, stopping health checks",
			"endpoint", job.endpoint.GetURLString())
		return
	}

	if repoErr := wp.repository.UpdateEndpoint(job.ctx, job.endpoint); repoErr != nil {
		var notFoundErr *domain.ErrEndpointNotFound
		if errors.As(repoErr, &notFoundErr) {
			// This shouldn't happen since we checked Exists() above, but handle it gracefully
			wp.logger.Debug("Endpoint not found during update, stopping health checks",
				"endpoint", job.endpoint.GetURLString())
			return
		}
		// Other repository errors should still be logged as errors
		wp.logger.Error("Failed to update endpoint",
			"endpoint", job.endpoint.GetURLString(),
			"error", repoErr)
		return
	}

	// Only reschedule if repository update succeeded
	scheduler.ScheduleCheck(job.endpoint, job.endpoint.NextCheckTime, job.ctx)

	shouldLog, errorCount := wp.statusTracker.ShouldLog(
		job.endpoint.GetURLString(),
		result.Status,
		err != nil)

	if shouldLog {
		if errorCount > 0 ||
			(result.Status == domain.StatusOffline ||
				result.Status == domain.StatusBusy ||
				result.Status == domain.StatusUnhealthy) {
			wp.logger.WarnWithEndpoint("Endpoint health issues for", job.endpoint.Name,
				"status", result.Status.String(),
				"consecutive_failures", errorCount,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		} else {
			wp.logger.InfoHealthStatus("Endpoint status changed for",
				job.endpoint.Name,
				result.Status,
				"latency", result.Latency,
				"next_check_in", nextInterval)
		}
	}
}