# go-cron
[![Build Status](https://travis-ci.org/ringtail/go-cron.svg?branch=master)](https://travis-ci.org/ringtail/go-cron)
[![Codecov](https://codecov.io/gh/ringtail/go-cron/branch/master/graph/badge.svg)](https://codecov.io/gh/ringtail/go-cron)    

a cron library for go.

## Why create another cron lib 
<a href="github.com/robfig/cron">robfig/cron</a> is a very great library for go. But it does have some missing parts.For example: 
```golang 
c := cron.New()
c.AddFunc("0 30 * * * *", func() { fmt.Println("Every hour on the half hour") })
c.AddFunc("@hourly",      func() { fmt.Println("Every hour") })
c.AddFunc("@every 1h30m", func() { fmt.Println("Every hour thirty") })
c.Start()
..
// Funcs are invoked in their own goroutine, asynchronously.
...
// Funcs may also be added to a running Cron
c.AddFunc("@daily", func() { fmt.Println("Every day") })
..
// Inspect the cron job entries' next and previous run times.
inspect(c.Entries())
..
c.Stop()  // Stop the scheduler (does not stop any jobs already running).
```
The code above is how `robfig/cron` works. But it's impossible if you want to update a spec of job or remove a specific job. So for these reasons, I decided to create another cron lib based on <a href="github.com/robfig/cron">robfig/cron</a>. 

## Usage 
// TODO 

## *License*
This software is released under the Apache 2.0 license.
