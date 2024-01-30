/*
 * Copyright Skyramp Authors 2024
 */
package types

import "time"

const (
	// Skyramp worker image name
	SkyrampWorkerImage = "public.ecr.aws/j1n2c2p2/rampup/worker"
	// Skyramp worker image tag
	SkyrampWorkerImageTag = "latest"

	DefaultWorkerConfigDirPath = "/etc/skyramp"
	WorkerManagementPort       = 35142
	WorkerContainerName        = "skyramp-worker"

	WorkerReadyzPath     = "readyz"
	WorkerTestPath       = "skyramp/test"
	WorkerTestsPath      = "skyramp/tests"
	WorkerMockConfigPath = "skyramp/mocks"

	SkyrampLabelKey = "app.kubernetes.io/name"

	WorkerWaitTime = 2 * time.Minute

	KindNodeImage = "kindest/node:v1.27.3"
)
