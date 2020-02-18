package cronhorizontalpodautoscaler

import (
	log "github.com/Sirupsen/logrus"
	"github.com/ringtail/go-cron"
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
