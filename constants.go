package main

import "time"

const (
	tokenTTL          = 8999 * time.Hour
	queuePollInterval = 1 * time.Minute
	queueBatchSize    = 5
	queueConcurrency  = 4
	passwordResetTTL  = 1 * time.Hour
)
