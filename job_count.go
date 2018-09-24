package main

import (
	"fmt"
	"time"

	datadog "github.com/zorkian/go-datadog-api"
)

type jobCount struct {
	VcsType  string
	Username string
	Reponame string
	Branch   string
	Count    int
}

type jobCounts struct {
	jobCounts map[string]*jobCount
}

func newJobCounts() *jobCounts {
	return &jobCounts{
		jobCounts: make(map[string]*jobCount),
	}
}

func (o *jobCounts) ensure(job *circleCiJob) *jobCount {
	key := job.toKey()
	if jc, ok := o.jobCounts[key]; ok {
		return jc
	}

	jc := &jobCount{
		VcsType:  job.VcsType,
		Username: job.Username,
		Reponame: job.Reponame,
		Branch:   job.Branch,
		Count:    0,
	}
	o.jobCounts[key] = jc

	return jc
}

func (o *jobCounts) incr(job *circleCiJob) {
	jc := o.ensure(job)
	jc.Count++
}

func (o *jobCounts) toMetrics(now time.Time, metricName string) []datadog.Metric {
	metrics := make([]datadog.Metric, len(o.jobCounts))
	timestamp := float64(now.Unix())

	i := 0
	for _, jobCount := range o.jobCounts {
		count := float64(jobCount.Count)
		metrics[i] = datadog.Metric{
			Metric: &metricName,
			Points: []datadog.DataPoint{{&timestamp, &count}},
			Tags: []string{
				fmt.Sprintf("vcs_type:%s", jobCount.VcsType),
				fmt.Sprintf("username:%s", jobCount.Username),
				fmt.Sprintf("reponame:%s", jobCount.Reponame),
				fmt.Sprintf("branch:%s", jobCount.Branch),
			},
		}
		i++
	}

	return metrics
}

func (o *jobCounts) getTotalCount() int {
	cnt := 0

	for _, jobCount := range o.jobCounts {
		cnt += jobCount.Count
	}

	return cnt
}
