package controller

type Interface interface {
	Run(workerCount int, stopCh chan struct{})
	IsReady() bool
}
