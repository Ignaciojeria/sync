package dev

import (
	"gitinittest5/internal/quality/application/test_report"
	"gitinittest5/internal/shared/server"
)

// Register wires quality (test report) routes onto the shared server.
func Register(s *server.Server, runner *testreport.Runner) {
	testReportPageHandler(s)
	testReportCoverageHandler(s)
	testReportRunHandler(s, runner)
}
