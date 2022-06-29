package controller

import (
	"github.com/ringtail/go-cron"
	log "k8s.io/klog/v2"
	"sort"
	"time"
)

const (
	maxOutOfDateTimeout = time.Minute * 5
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
	ReDoMissingJobs(map[string]CronJob)
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
			// clean up out of date jobs when it reach maxOutOfDateTimeout
			if e.Next.Add(maxOutOfDateTimeout).After(time.Now()) {
				return true
			}
			log.Warningf("The job %s is out of date and need to be clean up.", job.Name())
		}
	}
	return false
}

func (ce *CronHPAExecutor) ReDoMissingJobs(jobQueue map[string]CronJob) {
	reDoEntriesMap := map[*TargetRef][]*cron.Entry{}
	entries := ce.Engine.Entries()
	for _, job := range jobQueue {
		for _, e := range entries {
			if e.Job.ID() == job.ID() {
				if e.Next.Add(maxOutOfDateTimeout).Before(time.Now()) {
					if e.Next.IsZero() {
						continue
					}
					if _, ok := reDoEntriesMap[job.Ref()]; !ok {
						reDoEntriesMap[job.Ref()] = []*cron.Entry{}
					}
					reDoEntriesMap[job.Ref()] = append(reDoEntriesMap[job.Ref()], e)

				}
			}
		}
	}
	// sort job
	for target, reDoEntries := range reDoEntriesMap {
		log.Infof("%s %s/%s has %d redo jobs", target.RefKind, target.RefNamespace, target.RefName, len(reDoEntries))
		if len(reDoEntries) > 1 {
			sort.Sort(cron.ByTime(reDoEntries))
		}
		// redo
		for _, entry := range reDoEntries {
			log.Infof("%s %s/%s doing redo job: %s", target.RefKind, target.RefNamespace, target.RefName, entry.Job.ID())
			if msg, err := entry.Job.Run(); err != nil {
				log.Warningf("failed to redo missing job %s, msg %s, err %v", entry.Job.ID(), msg, err)
			}
			log.Infof("%s %s/%s succeed redo job: %s", target.RefKind, target.RefNamespace, target.RefName, entry.Job.ID())
		}
	}

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
