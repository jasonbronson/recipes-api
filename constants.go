package main

import "time"

const (
	tokenTTL          = 24 * time.Hour
	queuePollInterval = 1 * time.Minute
	queueBatchSize    = 5
	queueConcurrency  = 4
	passwordResetTTL  = 1 * time.Hour
)
