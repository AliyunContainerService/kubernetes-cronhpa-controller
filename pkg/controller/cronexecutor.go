package controller

import (
	"github.com/ringtail/go-cron"
	log "k8s.io/klog/v2"
	"time"
)

type CronConfig struct {
	Timezone *time.Location
}

type CronExecutor interface {
	Run()
	Stop()
	AddJob(job CronJob) error
	Update(job CronJob) error
	RemoveJob(job CronJob) error
	FindJob(job CronJob) bool
	ListEntries() []*cron.Entry
}

type CronHPAExecutor struct {
	Engine *cron.Cron
}

func (ce *CronHPAExecutor) AddJob(job CronJob) error {
	err := ce.Engine.AddJob(job.SchedulePlan(), job)
	if err != nil {
		log.Errorf("Failed to add job to engine,because of %v", err)
	}
	return err
}

func (ce *CronHPAExecutor) ListEntries() []*cron.Entry {
	entries := ce.Engine.Entries()
	return entries
}

func (ce *CronHPAExecutor) FindJob(job CronJob) bool {
	entries := ce.Engine.Entries()
	for _, e := range entries {
		if e.Job.ID() == job.ID() {
			if e.Next.After(time.Now()) {
				return true
			}
			log.Warningf("The job %s is out of date and need to be clean up.", job.Name())
		}
	}
	return false
}

func (ce *CronHPAExecutor) Update(job CronJob) error {
	ce.Engine.RemoveJob(job.ID())
	err := ce.Engine.AddJob(job.SchedulePlan(), job)
	if err != nil {
		log.Errorf("Failed to update job to engine,because of %v", err)
	}
	return err
}

func (ce *CronHPAExecutor) RemoveJob(job CronJob) error {
	ce.Engine.RemoveJob(job.ID())
	return nil
}

func (ce *CronHPAExecutor) Run() {
	ce.Engine.Start()
}

func (ce *CronHPAExecutor) Stop() {
	ce.Engine.Stop()
}

func NewCronHPAExecutor(timezone *time.Location, handler func(job *cron.JobResult)) CronExecutor {
	if timezone == nil {
		timezone = time.Now().Location()
	}
	c := &CronHPAExecutor{
		Engine: cron.NewWithLocation(timezone),
	}
	c.Engine.AddResultHandler(handler)
	return c
}
