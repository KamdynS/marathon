//go:build !adapters_sqs
// +build !adapters_sqs

// Package sqsqueue provides an SQS-backed queue adapter.
// This stub is built when the adapters_sqs build tag is not enabled so that
// editors and linters can still recognize the package and avoid "no packages
// found for open file" errors.
package sqsqueue

const _adapterDisabled = true
