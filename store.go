// Copyright 2016-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://vaibhav-sinha.mit-license.org/license.txt for details.

package jobqueue

import (
	"context"
	"errors"
)

var (
	// ErrNotFound must be returned from Store implementation when a certain job
	// could not be found in the specific data store.
	ErrNotFound = errors.New("jobqueue: job not found")
)

// Store implements persistent storage of jobs.
type Store interface {
	// Start is called when the manager starts up.
	// This is a good time for cleanup. E.g. a persistent store might moved
	// crashed jobs from a previous run into the Failed state.
	Start(StartupBehaviour) error

	// Create adds a job to the store.
	Create(context.Context, *Job) error

	// Delete removes a job from the store.
	Delete(context.Context, *Job) error

	// Update updates a job in the store. This is called frequently as jobs
	// are processed. Update must allow for concurrent updates, e.g. by locking.
	Update(context.Context, *Job) error

	// Next picks the next job to execute.
	//
	// The store should take the job priorities into account when picking the
	// next job. Jobs with higher priorities should be executed first.
	//
	// If no job is ready to be executed, e.g. the job queue is idle, the
	// store must return nil for both the job and the error.
	Next() (*Job, error)

	// Stats returns statistics about the store, e.g. the number of jobs
	// waiting, working, succeeded, and failed. This is run when the manager
	// starts up to get initial stats.
	Stats(context.Context, *StatsRequest) (*Stats, error)

	// Lookup returns the details of a job by its identifier.
	// If the job could not be found, ErrNotFound must be returned.
	Lookup(context.Context, string) (*Job, error)

	// LookupByCorrelationID returns the details of jobs by their correlation identifier.
	// If no such job could be found, an empty array is returned.
	LookupByCorrelationID(context.Context, string) ([]*Job, error)

	// List returns a list of jobs filtered by the ListRequest.
	List(context.Context, *ListRequest) (*ListResponse, error)
}

// StatsRequest returns information about the number of managed jobs.
type StatsRequest struct {
	Topic            string // filter by topic
	CorrelationGroup string // filter by correlation group
}

// ListRequest specifies a filter for listing jobs.
type ListRequest struct {
	Topic            string // filter by topic
	CorrelationGroup string // filter by correlation group
	CorrelationID    string // filter by correlation identifier
	State            string // filter by job state
	Limit            int    // maximum number of jobs to return
	Offset           int    // number of jobs to skip (for pagination)
}

// ListResponse is the outcome of invoking List on the Store.
type ListResponse struct {
	Total int    // total number of jobs found, excluding pagination
	Jobs  []*Job // list of jobs
}
