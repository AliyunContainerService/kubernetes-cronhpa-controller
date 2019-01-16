package cronhorizontalpodautoscaler

type CronProcessor interface {
	Process(spec string) CronJob
}

type CronProcessorParser struct {
}
